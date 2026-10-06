# Arquitetura

Este documento explica **como o backend está organizado e por quê**, e como o frontend (etapa 2) se encaixa. A filosofia é a de
Go: pacotes por assunto, interfaces pequenas **declaradas por quem as usa**, sem camadas cerimoniais. As decisões estão em
[docs/adr](adr).

## 1. Visão geral

```mermaid
flowchart LR
    subgraph Dev["desenvolvimento (origens diferentes)"]
        direction TB
        VITE["React + Vite<br/>:5173 (etapa 2)"]
    end
    subgraph Prod["produção (mesma origem)"]
        direction TB
        NGINX["nginx<br/>serve o SPA e faz proxy de /api (etapa 2)"]
    end

    VITE -- "HTTP + CORS" --> API
    NGINX -- "proxy /api → :8080" --> API

    subgraph API["backend Go — processo único (cmd/api → internal/app)"]
        direction TB
        MW["observe (request ID, slog) · CORS · recover<br/>timeout · limite de body"] --> MUX["ServeMux<br/>(tabela de rotas)"]
        MUX --> RL["rate limit<br/>(IP ou usuário)"] --> AU["authenticate + papel"] --> H["handlers finos<br/>httpapi"]
        H --> CAT["catalog.Service"]
        H --> CUS["customer.Service"]
        H --> BK["booking.Service<br/>regras + snapshot + máquina de estados"]
        H --> DB["dashboard.Service<br/>KPIs, série, ranking"]
        H --> AS["auth.Service<br/>bcrypt · JWT · refresh"]
        BK -. "catalog.Item / customer.Customer (leitura)" .-> CAT
        BK -. "…" .-> CUS
    end

    CAT -. "catalog.Repository" .-> PG
    CUS -. "customer.Repository" .-> PG
    BK -. "booking.Repository" .-> PG
    DB -. "dashboard.Reader" .-> PG
    AS -. "auth.Store" .-> PG
    PG[("PostgreSQL 16<br/>pgx · migrations embutidas<br/>EXCLUDE USING gist")]
    MEM[("memstore<br/>(testes)")]
    CAT & CUS & BK & DB & AS -. "mesmas interfaces" .-> MEM
    DOCS["/docs (Swagger UI) · /openapi.json<br/>embutidos no binário"] --- MUX
```

| Pacote | Papel | Importa (do projeto) |
|---|---|---|
| `cmd/api` | `main`: flags, sinais, `os.Exit`; embute `time/tzdata` | `app`, `config` |
| `internal/app` | composition root: liga tudo, `Serve` com *graceful shutdown*, `Healthcheck` | todos |
| `internal/config` | variáveis de ambiente → `Config`; recusa segredo fraco em `release`; fuso e CORS | — |
| `internal/catalog` | catálogo de serviços: modelo, validação, `Service`, `Repository` | `validation` |
| `internal/customer` | cadastro de clientes (e-mail único) | `validation` |
| `internal/booking` | **agendamentos**: máquina de estados, snapshot, regras, `Repository` | `catalog`, `customer`, `period`, `validation` |
| `internal/dashboard` | definições dos indicadores, série zero-preenchida, `Reader` | `booking`, `period`, `validation` |
| `internal/period` | `from`/`to` (datas inclusivas, fuso do negócio) → intervalo `[From, To)` | `validation` |
| `internal/auth` | usuários, bcrypt, JWT, refresh com rotação, papéis, `Store` | `validation` |
| `internal/httpapi` | rotas, middleware, JSON, RFC 9457, CORS, OpenAPI + Swagger UI | todos os de domínio, `ratelimit` |
| `internal/postgres` | `pgx`, migrations embutidas; implementa os `Repository`/`Reader`/`Store` | domínio |
| `internal/memstore` | implementações em memória (testes) sobre um `DB` compartilhado | domínio |
| `internal/repotest`, `internal/testdb` | suítes de contrato compartilhadas; schema isolado por teste | — |
| `internal/ratelimit`, `internal/validation` | token bucket; erros por campo, e-mail, dígitos | — |

**Regra de dependência:** o domínio (`catalog`, `customer`, `booking`, `dashboard`, `auth`) não importa `httpapi`, `postgres` nem `pgx`.
`httpapi` não importa `postgres`. Só `app` conhece todos. O compilador recusa ciclos.

## 2. Modelo de dados

```mermaid
erDiagram
    services ||--o{ appointments : "service_id (RESTRICT)"
    customers ||--o{ appointments : "customer_id (RESTRICT)"
    users ||--o{ refresh_tokens : "user_id (CASCADE)"
    services {
        uuid id PK
        text name
        int duration_min "5..480"
        int price_cents ">= 0"
        bool active
    }
    customers {
        uuid id PK
        text name
        text email UK
        text phone
        text notes
        timestamptz created_at
    }
    appointments {
        uuid id PK
        text service_name "snapshot"
        int duration_min "snapshot"
        int price_cents "snapshot"
        timestamptz starts_at
        timestamptz ends_at "starts_at + duration"
        text status "scheduled|completed|cancelled|no_show"
    }
    users {
        uuid id PK
        text email UK
        text role "admin|staff"
    }
```

`appointments` carrega a restrição `appointments_no_overlap` ([ADR 0002](adr/0002-non-overlapping-agenda.md)) e os índices do
dashboard ([ADR 0004](adr/0004-dashboard-aggregations.md)). As migrações são SQL puro, embutidas no binário, aplicadas na partida com
`pg_advisory_lock` (várias instâncias subindo juntas aplicam uma de cada vez).

## 3. Fluxo: agendar (e por que não há corrida)

`POST /v1/appointments` por um `staff`:

```mermaid
sequenceDiagram
    autonumber
    participant C as Cliente HTTP
    participant H as httpapi.createAppointment
    participant B as booking.Service
    participant K as catalog / customer
    participant R as postgres.Appointments
    participant P as PostgreSQL

    C->>H: POST {customer_id, service_id, starts_at}
    Note over H: observe → CORS → recover → limits → rate limit → authenticate
    H->>H: decode (JSON estrito)
    H->>B: Create(input)
    B->>B: valida: início no futuro, ids, notas
    B->>K: Get(customer), Get(service)
    K-->>B: cliente + serviço (ativo?)
    B->>B: snapshot: nome, duração, preço; ends_at = início + duração
    B->>R: Create(appointment)
    R->>P: INSERT … (checa EXCLUDE no GiST, atomicamente)
    alt horário livre
        P-->>R: ok
        R-->>H: ok → 201 + Location
    else sobreposição (SQLSTATE 23P01)
        P-->>R: exclusion_violation
        R-->>B: ErrSlotTaken
        B-->>H: ErrSlotTaken → 409 slot_unavailable (RFC 9457)
    end
```

## 4. Máquina de estados

```mermaid
stateDiagram-v2
    [*] --> scheduled: POST /v1/appointments
    scheduled --> completed: PATCH status (após o início)
    scheduled --> no_show: PATCH status (após o início)
    scheduled --> cancelled: PATCH status (a qualquer momento)
    completed --> [*]
    no_show --> [*]
    cancelled --> [*]
```

- `CanTransitionTo` é a máquina inteira (3 linhas): só `scheduled` se move; os outros três estados são terminais.
- `completed`/`no_show` antes do horário de início → `409 not_started`. Qualquer outra transição → `409 invalid_transition`.
- A escrita é um *compare-and-set* (`UPDATE … WHERE id = $1 AND status = $2`): duas transições simultâneas não se atropelam.

## 5. Fluxo de uma requisição e mapa de erros

Ordem dos *middlewares* (de fora para dentro): `observe` (request ID, cabeçalho `nosniff`, *access log* `slog`) → `cors` (o *preflight*
responde aqui, sem autenticação nem rate limit) → `recoverer` (pânico → 500) → `limits` (timeout de 8 s e corpo de 1 MiB) → `ServeMux` →
`protect` por rota (rate limit → autenticação → papel) → handler fino.

`server.fail` é o **único** tradutor erro → HTTP (RFC 9457, `type: about:blank`, `code` estável, `request_id`):

| Erro de domínio | Status | `code` |
|---|---|---|
| `validation.Errors` | 422 | `validation_failed` (+ `errors[{field,message}]`) |
| JSON inválido / campo desconhecido | 400 | `invalid_json` |
| corpo > 1 MiB / tipo ≠ JSON | 413 / 415 | `body_too_large` / `unsupported_media_type` |
| `catalog/customer/booking.ErrNotFound` (inclui id malformado) | 404 | `service_not_found` / `customer_not_found` / `appointment_not_found` |
| `ErrEmailTaken` | 409 | `email_taken` |
| `booking.ErrSlotTaken` | 409 | `slot_unavailable` |
| `booking.ErrInvalidTransition` / `ErrNotStarted` | 409 | `invalid_transition` / `not_started` |
| `catalog.ErrInUse` / `customer.ErrInUse` | 409 | `service_in_use` / `customer_in_use` |
| `booking.ErrUnknownReference` (cliente/serviço removido entre a leitura e a escrita) | 409 | `unknown_reference` |
| `booking.ErrServiceInactive` | 422 | `service_inactive` |
| `auth.ErrInvalidCredentials` / `ErrInvalidToken` / sem token | 401 | `invalid_credentials` / `invalid_token` / `missing_token` |
| papel insuficiente | 403 | `forbidden` |
| rate limit | 429 | `rate_limited` (+ `Retry-After`) |
| `context.DeadlineExceeded` | 503 | `timeout` |
| qualquer outro | 500 | `internal_error` (a causa só vai para o log) |

## 6. Contrato OpenAPI que nunca diverge

`internal/httpapi/openapi.json` (3.0.3) é embutido no binário e servido em `/openapi.json`, com Swagger UI em `/docs/` (assets
vendorizados, sem CDN; carrega a spec por caminho **relativo**, então funciona também sob `/api/docs/` atrás do nginx). Três testes
o mantêm fiel — sem banco e sem `.env`:

1. `TestOpenAPIMatchesRouter`: compara a tabela `routes()` com a spec (todo endpoint existe nos dois lados; segurança, 401/403/429, corpo, `operationId` único, `404` em `{id}`).
2. Todo teste de API passa por `env.do`, que **valida cada resposta contra a spec**: status declarado, `Content-Type` declarado e corpo conforme o schema (tipos,
   obrigatórios, **campos não declarados são erro**, formatos `date-time`/`date`/`uuid`).
3. `TestOpenAPIReferencesResolve` (nenhum `$ref` quebrado, nenhum componente morto) e `TestOpenAPIExamplesMatchTheirSchemas` (os exemplos que o frontend vai copiar são válidos).

## 7. Como o frontend (etapa 2) se conecta

- **Dev:** SPA em `http://localhost:5173` chamando `http://localhost:8095`. A API já vem com `CORS_ALLOWED_ORIGINS=http://localhost:5173`.
- **Produção:** o `docker-compose.yml` tem o serviço `frontend` (nginx) **comentado**, pronto para a etapa 2: nginx serve o SPA e
  `location /api/ { proxy_pass http://api:8080/; }`. Mesma origem, sem CORS.
- **Contrato:** `GET /openapi.json` (ou `backend/internal/httpapi/openapi.json`) é a fonte de verdade — dá para gerar tipos TypeScript
  (ex.: `openapi-typescript`). Valores monetários são **centavos inteiros**; instantes são RFC 3339 UTC; dias de calendário (`from`/`to`,
  séries) são `YYYY-MM-DD` no fuso do negócio, devolvido no campo `timezone`.

## 8. Como adicionar um recurso (passo a passo)

1. Crie `internal/<recurso>/` com o modelo, o `Service` (validação e regras) e a interface `Repository`, e erros de domínio.
2. Implemente no `internal/memstore` e no `internal/postgres` (+ migração `000N_*.sql`) e **escreva a suíte em `internal/repotest`**,
   rodando-a nos dois (`memstore_test.go` e `postgres_test.go`).
3. Em `internal/httpapi`: declare a interface do serviço em `server.go`, acrescente linhas em `routes()`, escreva os handlers, um `case` em `fail`.
4. Atualize `openapi.json` (paths + schemas + exemplos): os testes de contrato dizem exatamente o que faltou.
5. Ligue no `internal/app/app.go`.
