# go-react-business-suite

[![CI](https://github.com/andreccls/go-react-business-suite/actions/workflows/ci.yml/badge.svg)](https://github.com/andreccls/go-react-business-suite/actions/workflows/ci.yml)
[![Go 1.24](https://img.shields.io/badge/Go-1.24-00ADD8)](https://go.dev/)
[![PostgreSQL 16](https://img.shields.io/badge/PostgreSQL-16-4169E1)](https://www.postgresql.org/)
[![OpenAPI 3.0](https://img.shields.io/badge/OpenAPI-3.0-6BA539)](backend/internal/httpapi/openapi.json)
[![React 19](https://img.shields.io/badge/React-19-61DAFB)](frontend)
[![TypeScript strict](https://img.shields.io/badge/TypeScript-strict-3178C6)](frontend/tsconfig.json)
[![Coverage backend](https://img.shields.io/badge/backend%20coverage-97.7%25-brightgreen)](docs/TESTING.md)
[![Coverage frontend](https://img.shields.io/badge/frontend%20coverage-99.9%25-brightgreen)](docs/TESTING.md#frontend)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

> ⚠️ **PROJETO DE EXEMPLO DE CÓDIGO (reference / sample) — NÃO É UM PRODUTO PRONTO PARA PRODUÇÃO.**
> Existe para demonstrar **uma solução completa para um negócio pequeno** (catálogo de serviços, clientes, agenda e painel de
> resultados) com backend em **Go** e frontend em **React + TypeScript** — junto com a documentação de arquitetura, escolhas técnicas e testes.
> O estúdio e os dados são **fictícios**. Segredos e senhas do `docker-compose.yml`/`.env.example` são **apenas de desenvolvimento**.
> Não o publique na internet como está: a lista honesta do que falta está em [Limitações conhecidas](#limitações-conhecidas-e-próximos-passos).

> 🇬🇧 English summary: [README.en.md](README.en.md).

## O que é

**Studio Suite** — o sistema de um estúdio/clínica pequeno e fictício, com **um único prestador** (uma agenda):

- **Catálogo de serviços:** nome, descrição, duração (min), **preço em centavos**, ativo/inativo.
- **Clientes:** nome, e-mail único, telefone, observações.
- **Agenda:** cliente + serviço + início; o fim é **derivado** da duração do serviço, que junto com o preço fica **congelado (snapshot)** no
  agendamento. Sem sobreposição de horários, sem agendar no passado nem serviço inativo, com máquina de estados
  `scheduled → completed | cancelled | no_show`.
- **Painel de resultados (dashboard):** KPIs do período (receita realizada, agendamentos por status, taxas de cancelamento/no-show,
  ticket médio, novos clientes), série diária, top serviços e próximos agendamentos.
- **Acesso:** login com JWT curto + refresh rotativo; papéis `admin` e `staff` (staff não remove nada).

Foi desenhado com **SOLID idiomático, KISS e YAGNI**: só a stdlib para HTTP, 4 dependências diretas, sem ORM nem framework — cada escolha
está num [ADR](docs/adr). A regra mais delicada — **dois agendamentos nunca se sobrepõem, nem sob concorrência** — é garantida **pelo
banco** (restrição de exclusão do PostgreSQL) e provada por teste: 30 requisições simultâneas pelo mesmo horário → **1 × 201 e 29 × 409**.

## Telas

Geradas por `./e2e/run.sh screenshots` (Playwright, Chromium) a partir da pilha real, depois do `make demo`. Claro/escuro seguem `prefers-color-scheme`.

| | |
|---|---|
| ![Painel de resultados, tema claro](docs/images/01-painel-claro.png)<br/>**Painel** — KPIs do período, série diária (SVG próprio), top serviços | ![Painel de resultados, tema escuro](docs/images/02-painel-escuro.png)<br/>**Painel**, tema escuro |
| ![Agendamento recusado por conflito de horário (409)](docs/images/03-agenda-conflito.png)<br/>**Agenda** — o `409 slot_unavailable` explicado em português, com o diálogo mantido aberto | ![Lista de serviços, tema escuro](docs/images/04-servicos-escuro.png)<br/>**Serviços** (CRUD; "Excluir" só para admin) |
| ![Agenda no celular: cada linha vira um cartão](docs/images/05-agenda-celular.png)<br/>**Celular** — tabelas viram cartões | ![Novo agendamento no celular, tema escuro](docs/images/06-novo-agendamento-celular-escuro.png)<br/>**Novo agendamento** no celular, tema escuro |

## Arquitetura

```mermaid
flowchart LR
    U["Navegador"] -- "HTTP :8096" --> FE
    subgraph Stack["docker compose (projeto grbs)"]
        direction LR
        FE["frontend · nginx não-root<br/>SPA React + CSP + gzip<br/>fallback de rotas"] -- "/api/ → proxy (mesma origem, sem CORS)" --> API
        API["api · Go 1.24<br/>net/http · slog · pgx<br/>distroless, não-root"] --> PG[("PostgreSQL 16<br/>EXCLUDE USING gist<br/>índices do dashboard")]
    end
    DEV["Vite dev server<br/>(make frontend-dev)"] -. "proxy /api" .-> API
    API -. "embutido no binário" .- DOCS["/api/docs (Swagger UI)<br/>/api/openapi.json"]
    SPEC["openapi.json<br/>(contrato)"] -. "openapi-typescript<br/>+ check no CI" .-> TYPES["frontend/src/api/schema.d.ts"]
    SPEC -. "testes mantêm igual ao roteador" .- API
```

No navegador, o SPA guarda o **access token só em memória** e o **refresh token em `sessionStorage`**, renova com **uma fila única** de refresh e usa o
TanStack Query para estado de servidor (veja [ARCHITECTURE §9](docs/ARCHITECTURE.md#9-frontend-react--typescript--vite)).
Dentro da API, as interfaces (`booking.Repository`, `dashboard.Reader`, `auth.Store`…) são declaradas **no pacote que as consome**; o
domínio (`catalog`, `customer`, `booking`, `dashboard`, `auth`) não conhece HTTP nem SQL, e o mesmo conjunto de testes de contrato
roda contra o fake em memória e contra o PostgreSQL real. Diagramas detalhados (modelo de dados, fluxo de agendamento, máquina de estados,
mapa de erros) em [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Estrutura do monorepo

```
.
├── backend/                         # API Go
│   ├── cmd/api/main.go              # flags, sinais, os.Exit — nada de lógica
│   ├── internal/
│   │   ├── app/                     # composition root + Serve (graceful shutdown) + Healthcheck
│   │   ├── config/                  # env vars → Config; recusa segredo fraco em release
│   │   ├── catalog/ customer/       # catálogo e clientes: modelo, regras, Repository
│   │   ├── booking/                 # agendamentos: estados, snapshot, regras, Repository
│   │   ├── dashboard/ period/       # KPIs/série/ranking · período from/to no fuso do negócio
│   │   ├── auth/                    # usuários, bcrypt, JWT, refresh rotativo, papéis
│   │   ├── httpapi/                 # rotas, middleware, RFC 9457, CORS, openapi.json, swagger/ (embutidos)
│   │   ├── postgres/                # pgx + migrations/*.sql embutidas
│   │   ├── memstore/ repotest/ testdb/   # fakes · suítes de contrato · schema isolado por teste
│   │   └── ratelimit/ validation/   # token bucket · erros de campo, e-mail
│   ├── scripts/ (demo.sh, coverage-gate.sh)   Dockerfile (multi-stage, distroless)
├── frontend/                        # SPA React + TypeScript + Vite
│   ├── src/api/                     # cliente fino com refresh, endpoints e hooks (tipos GERADOS do openapi.json)
│   ├── src/auth/ lib/ components/   # sessão · formatação/fuso/validação · Modal, Field, gráfico SVG…
│   ├── src/pages/                   # Login, Painel, Agenda, Serviços, Clientes, Usuários
│   ├── src/test/                    # MSW + fake da API com estado, tipado pelo contrato
│   └── Dockerfile  nginx/           # build em 2 estágios → nginx não-root (CSP, gzip, proxy /api)
├── e2e/                             # Playwright (contêiner oficial) contra a pilha completa + gerador de screenshots
├── docs/ (ARCHITECTURE.md, TESTING.md, adr/)
└── docker-compose.yml  Makefile  .env.example  .github/workflows/ci.yml   docs/images/ (screenshots)
```

## Como rodar

Pré-requisito: **Docker** + `make` (e `curl`/`jq` para o demo). **Não precisa de Go nem de Node no host** — tudo roda em contêineres, e um clone limpo
funciona **sem `.env`**.

```bash
make up      # PostgreSQL 16 + API + frontend (nginx)  ->  app: http://localhost:8096   |   API: http://localhost:8095  (Swagger: /docs/ ou /api/docs/ pelo app)
make demo    # passeio com curl (agenda vazia): popula histórico e confere os KPIs com um cálculo manual — depois abra o painel no app
make test    # backend (unit + PostgreSQL + concorrência, -race, gate) E frontend (Vitest + gate de cobertura)
make lint    # gofmt + go vet + staticcheck  E  ESLint (jsx-a11y) + tsc strict + tipos gerados em dia
make e2e     # Playwright contra a pilha completa (precisa de `make up`; baixa a imagem oficial do Playwright, ~1,5 GB)
make frontend-dev   # Vite com HMR em http://localhost:5180 (proxy /api → API); também: frontend-test, frontend-coverage, frontend-build, frontend-types
make down    # para tudo (mantém o volume do banco)   |   make clean: apaga volumes/imagens/artefatos do projeto
```

**Entrar:** abra <http://localhost:8096> e use o admin de desenvolvimento do `.env.example` (`admin@example.com` / `dev-only-admin-password`, criado na partida a partir de
`ADMIN_EMAIL`/`ADMIN_PASSWORD`). Para ver o painel com números, rode `make demo` antes (ele exige agenda vazia, um `make up` novo). Pelo menu **Usuários** o admin cria contas `staff`
(que não veem "Excluir" nem o menu de usuários).

Portas de host (em `127.0.0.1`, configuráveis via `.env`): **web 8096** (`WEB_PORT`), API **8095** (`API_PORT`), PostgreSQL **5437**, Vite dev **5180** (`DEV_PORT`); projeto compose
`grbs` (isola contêineres e volumes de outros repositórios). O fuso do negócio (`BUSINESS_TZ`, padrão `America/Sao_Paulo`) vai para a API **e** para o *build* do front (`VITE_BUSINESS_TZ`).

### Configuração (variáveis de ambiente)

| Variável | Padrão | Descrição |
|---|---|---|
| `APP_ENV` | `release` | `release` (padrão, **estrito**) ou `development`. Em `release` a API **recusa subir** com `JWT_SECRET` < 32 caracteres, de baixa entropia ou parecido com placeholder, e com `ADMIN_PASSWORD` fraca. |
| `DATABASE_URL` | — (obrigatória) | URL do PostgreSQL |
| `JWT_SECRET` | — (obrigatória) | segredo HS256 (≥ 16 em `development`, ≥ 32 e não-placeholder em `release`) |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | — | cria o admin inicial (idempotente); devem vir juntos |
| `BUSINESS_TZ` | `UTC` (o compose usa `America/Sao_Paulo`) | fuso IANA que define o "dia" dos filtros e do dashboard |
| `CORS_ALLOWED_ORIGINS` | vazio (CORS desligado) | origens permitidas, separadas por vírgula (`*` = qualquer). Em dev: `http://localhost:5173`. Em produção, nginx faz proxy e não precisa |
| `HTTP_ADDR` | `:8080` | endereço de escuta |
| `JWT_ACCESS_TTL` / `JWT_REFRESH_TTL` | `15m` / `168h` | validade dos tokens |
| `RATE_LIMIT_AUTH_PER_MIN` / `RATE_LIMIT_API_PER_MIN` | `10` / `120` | por IP em `/v1/auth/*` · por usuário nas demais rotas |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

### Evidência: a stack real por curl (saída do `make demo`, resumida)

Executado em 2026-10-06 contra a stack de `make up` (imagem distroless + PostgreSQL 16), a partir de um banco vazio:

```text
== auth
401  protected route without token (401 missing_token)      401  login with a wrong password (401 invalid_credentials)
200  login admin  (expires_in=900s)     200  refresh (rotation)
401  reuse of the OLD refresh token (401, whole family revoked)
201  admin creates a staff user         403  staff cannot create users (403 forbidden)
== catalog and customers
422  invalid service  {"code":"validation_failed","errors":[{"field":"name",...},{"field":"duration_min",...},{"field":"price_cents",...}]}
409  duplicate e-mail (409 email_taken)
== booking rules (future agenda)
201  book Haircut 2026-10-08T14:00Z (30 min)   ends_at=2026-10-08T14:30:00Z price_cents=5000 status=scheduled
409  overlapping booking at 14:15   {"code":"slot_unavailable"}
201  back-to-back at 14:30 is fine
422  booking in the past            {"errors":[{"field":"starts_at","message":"must be in the future"}]}
422  inactive service               {"code":"service_inactive"}
409  same slot, other customer
== snapshot            PATCH Haircut price 5000 -> 9900 ... the appointment booked before still has price_cents=5000
== status transitions
409  complete a FUTURE appointment  {"code":"not_started"}      200  cancel      409  cancelled is terminal (invalid_transition)
201  the cancelled slot is free again          201  book Express check (5 min) starting in ~4 s
409  complete it before it starts (not_started)    ... sleep 5 ...    200  complete it after it started
```

**Os KPIs conferem com a soma manual.** O roteiro monta 19 agendamentos (14 de histórico inseridos por SQL — a API recusa o passado por desenho — e 5 criados pela
API) e calcula o esperado por conta própria a partir da mesma tabela, antes de consultar `GET /v1/dashboard/summary`:

```text
manual: total=19 scheduled=3 completed=11 cancelled=3 no_show=2 revenue=119000 ticket=10818
manual: revenue = 5000+20000+12000+20000+5000+12000+5000+5000+12000+20000+3000 = 119000      (11 agendamentos completed)
   ok   appointments_total                 expected 19       got 19
   ok   by_status  scheduled/completed/cancelled/no_show   expected 3/11/3/2    got 3/11/3/2
   ok   revenue_cents (completed only)     expected 119000   got 119000
   ok   average_ticket_cents               expected 10818    got 10818          (119000 / 11 = 10818,18)
   ok   cancellation_rate (3/19)           expected 0.1579   got 0.1579
   ok   no_show_rate (2/19)                expected 0.1053   got 0.1053
   ok   new_customers (4 created today)    expected 4        got 4
   ok   yesterday appointments / revenue   expected 2 / 25000 (5000+20000)       got 2 / 25000
   ok   daily series: 29 points, no holes; sum of the daily revenue = 119000
   ok   top service by realized revenue: Hair color (3 completed x 20000 = 60000), then Deep tissue massage 36000, Haircut 20000, Express check 3000
== demo finished: all dashboard numbers match the manual computation
```

Também verificados à mão: com `APP_ENV` não definido e os segredos de desenvolvimento, o contêiner **sai com código 1** listando
`JWT_SECRET is too weak for release mode` e `ADMIN_PASSWORD is too weak…`; `docker stop` (SIGTERM) loga `shutting down` e sai com código 0; o
*preflight* CORS (`OPTIONS` com `Origin: http://localhost:5173`) responde `204` com `Access-Control-Allow-Origin`, e uma origem não listada não recebe cabeçalhos CORS; a
imagem final roda como `nonroot:nonroot` (~5 MB).

Fluxo mínimo à mão:

```bash
API=http://localhost:8095; H='Content-Type: application/json'
TOKEN=$(curl -s -H "$H" $API/v1/auth/login -d '{"email":"admin@example.com","password":"dev-only-admin-password"}' | jq -r .access_token)
A="Authorization: Bearer $TOKEN"
SVC=$(curl -s -H "$A" -H "$H" $API/v1/services  -d '{"name":"Haircut","duration_min":30,"price_cents":5000}' | jq -r .id)
CUS=$(curl -s -H "$A" -H "$H" $API/v1/customers -d '{"name":"Ana Souza","email":"ana@example.com"}' | jq -r .id)
curl -s -H "$A" -H "$H" $API/v1/appointments -d "{\"customer_id\":\"$CUS\",\"service_id\":\"$SVC\",\"starts_at\":\"2030-01-07T14:00:00-03:00\"}"
curl -s -H "$A" "$API/v1/dashboard/summary?from=2030-01-01&to=2030-01-31"
```

## Endpoints

Contrato completo (schemas, exemplos, respostas) no Swagger UI em `/docs/` e em
[`backend/internal/httpapi/openapi.json`](backend/internal/httpapi/openapi.json) — **a fonte de verdade para o frontend**. Um teste falha se ele divergir do roteador.

| Método e rota | Acesso | Descrição | Respostas de erro |
|---|---|---|---|
| `POST /v1/auth/login` | público · rate limit/IP | `access_token` (JWT, 15 min) + `refresh_token` | 400 · 401 · 429 |
| `POST /v1/auth/refresh` | público · rate limit/IP | rotaciona o refresh (uso único; reuso revoga a família) | 400 · 401 · 429 |
| `POST /v1/auth/logout` | público · rate limit/IP | revoga um refresh token (204) | 400 · 429 |
| `GET /v1/auth/me` | autenticado | usuário atual (`id`, `email`, `role`) | 401 |
| `POST /v1/users` | **admin** | cria usuário (`admin` ou `staff`) | 403 · 409 · 422 |
| `POST /v1/services` · `GET /v1/services` | autenticado | cria · lista (`active`, `q`, `page`, `page_size`) | 422 |
| `GET /v1/services/{id}` · `PATCH` | autenticado | consulta · atualiza parcialmente (preço/duração não alteram o histórico) | 404 · 422 |
| `DELETE /v1/services/{id}` | **admin** | remove (se não houver agendamentos) | 403 · 404 · 409 `service_in_use` |
| `POST /v1/customers` · `GET /v1/customers` | autenticado | cria · lista (`q`, `page`, `page_size`) | 409 `email_taken` · 422 |
| `GET /v1/customers/{id}` · `PATCH` | autenticado | consulta · atualiza parcialmente | 404 · 409 · 422 |
| `DELETE /v1/customers/{id}` | **admin** | remove (se não houver agendamentos) | 403 · 404 · 409 `customer_in_use` |
| `POST /v1/appointments` | autenticado | **agenda** (snapshot, `ends_at` derivado) | 409 `slot_unavailable` · 422 (`service_inactive`, passado…) |
| `GET /v1/appointments` | autenticado | lista por início (`status`, `customer_id`, `from`, `to`, paginação) | 422 |
| `GET /v1/appointments/{id}` | autenticado | consulta | 404 |
| `PATCH /v1/appointments/{id}/status` | autenticado | `completed` · `no_show` · `cancelled` | 404 · 409 `invalid_transition`/`not_started` · 422 |
| `GET /v1/dashboard/summary` | autenticado | KPIs do período (`from`/`to`) | 422 |
| `GET /v1/dashboard/daily` | autenticado | série diária, zero-preenchida | 422 |
| `GET /v1/dashboard/top-services` | autenticado | ranking por receita realizada (`limit`) | 422 |
| `GET /v1/dashboard/upcoming` | autenticado | próximos agendamentos (`limit`) | 422 |
| `GET /healthz` · `GET /readyz` | público | liveness · readiness (consulta o banco) | 503 |
| `GET /docs/` · `GET /openapi.json` | público | Swagger UI e especificação (embutidos no binário) | — |

Todo `4xx/5xx` é `application/problem+json` (RFC 9457) com `code` estável e `request_id`; `422` traz `errors: [{field, message}]`; todas as
respostas levam `X-Request-ID`; `429` leva `Retry-After`. Listas respondem `{data, page, page_size, total}`. Dinheiro é **centavos inteiros**;
instantes são RFC 3339 UTC; `from`/`to` são datas `YYYY-MM-DD` **inclusivas** no fuso do negócio (máx. 366 dias).

### Regras de negócio → onde estão

| Regra | Onde / como é garantida |
|---|---|
| Horários não se sobrepõem (intervalo semiaberto; cancelado/no-show liberam) | `EXCLUDE USING gist` no PostgreSQL — [ADR 0002](docs/adr/0002-non-overlapping-agenda.md); teste de concorrência real |
| Não agendar no passado · não agendar serviço inativo | `booking.Service.Create` (422 `validation_failed` · 422 `service_inactive`) |
| `ends_at` = início + duração; preço/duração/nome congelados | `booking.Service.Create` — [ADR 0003](docs/adr/0003-price-duration-snapshot.md) |
| Transições válidas; `completed`/`no_show` só após o início | `Status.CanTransitionTo` + `Service.SetStatus`; escrita *compare-and-set* |
| Receita = soma dos `completed` (do snapshot); razões e ticket | `dashboard.Service` — [ADR 0004](docs/adr/0004-dashboard-aggregations.md) |
| `staff` não remove; só `admin` cria usuários | `AdminOnly` na tabela de rotas (`httpapi.routes`) |

## Testes

| Suíte | Resultado (medido em 2026-10-06) |
|---|---|
| **Backend** (Go, `-race`, PostgreSQL real) | **227** casos, 0 falhas · cobertura **97,7%** (gate ≥ 90%; domínio e serviços **100%**) · `go vet` e `staticcheck` limpos |
| **Frontend** (Vitest + Testing Library + MSW) | **188** casos em 15 arquivos, 0 falhas · cobertura **99,9%** linhas / 98,1% *branches* / 96,5% funções (gate ≥ 90%; `lib/` e `api/client.ts` com gate ≥ 95%, medem **100%** de linhas) · ESLint (jsx-a11y) e `tsc --noEmit` strict limpos |
| **E2E** (Playwright, Chromium, nginx + API + PostgreSQL reais) | **5** casos: fluxo completo com `409`, permissões de `staff` + reload, senha errada, **só teclado** + console sem erros, **sem rolagem horizontal no celular** |
| Tipos gerados × `openapi.json` | verificados no `make lint` e no CI (`check:api`) |
| *Bundle* de produção | JS **344,7 kB (106,5 kB gzip)** · CSS 9,4 kB (2,7 kB gzip) · imagem do front ~82 MB (nginx não-root) |

Estratégia completa, o que cada camada prova e exclusões justificadas em [docs/TESTING.md](docs/TESTING.md). Destaques:

- **Backend — contrato único, dois armazenamentos:** `internal/repotest` roda a mesma suíte no fake em memória e no PostgreSQL real (um *schema* por teste).
- **Backend — concorrência:** 25 criações do mesmo horário no repositório (1 vence), 24 criações sobrepostas escalonadas (nunca duas gravadas) e **30 `POST`
  simultâneos pela pilha completa → 1 × 201 + 29 × 409**.
- **Backend — OpenAPI como oráculo:** toda resposta de todo teste de API é validada contra o `openapi.json` (status, content-type, schema, sem campos extras).
- **Frontend — o mesmo contrato dos dois lados:** os tipos vêm do `openapi.json` e o *mock* do MSW é **tipado** por eles; mudar a spec quebra a compilação do front antes de qualquer teste.
- **Frontend — o refresh é o que mais importa:** `N` requisições simultâneas com `401` fazem **um** refresh; refresh **reusado/revogado** derruba a sessão; rede/`5xx` não. Tudo testado em `api/client.test.ts` (100%).
- **Frontend — fluxos reais:** agendar com `409`, *Concluir* só depois do início, permissões admin/staff, estados vazio/erro/carregando em toda tela, formatação `R$`/datas no fuso do negócio.
- O gate roda em `make test` e no CI; `REQUIRE_DB=1` impede que uma suíte de integração pulada passe como verde.

## Decisões técnicas

| Decisão | Escolha | Por quê (resumo) | ADR |
|---|---|---|---|
| HTTP no backend | stdlib `net/http` + `ServeMux`, `slog`, `pgx` (4 dependências diretas) | KISS; sem framework/ORM a esconder o que importa | [0001](docs/adr/0001-stdlib-vs-framework.md) |
| Agenda sem sobreposição | restrição `EXCLUDE USING gist` no PostgreSQL | correto sob concorrência por construção, sem locks | [0002](docs/adr/0002-non-overlapping-agenda.md) |
| Preço/duração no agendamento | *snapshot* copiado ao agendar | o catálogo muda; o histórico e a receita não | [0003](docs/adr/0003-price-duration-snapshot.md) |
| Dashboard | agregações SQL diretas (com `EXPLAIN`) | simples e rápido para o volume de uma agenda | [0004](docs/adr/0004-dashboard-aggregations.md) |
| Autenticação | JWT 15 min + refresh rotativo com detecção de reuso; papéis `admin`/`staff` | roubo de refresh é detectável; sem sessão no servidor | [0005](docs/adr/0005-auth-jwt-refresh.md) |
| Estado de servidor no front | TanStack Query (leituras por *key*, escritas invalidam) | cache/erro/loading prontos; sem Redux para ~6 telas | [0006](docs/adr/0006-server-state-tanstack-query.md) |
| Tipos do front | **gerados** do `openapi.json` e conferidos no CI; mock tipado pelo contrato | a spec é a única fonte da verdade | [0007](docs/adr/0007-generated-api-types.md) |
| Tokens no navegador | access **em memória**, refresh em `sessionStorage`; refresh de **fila única** | sem corrida entre abas (refresh é de uso único); limitação de XSS documentada | [0008](docs/adr/0008-token-storage.md) |
| Servir o front | nginx **não-root**: SPA + proxy `/api` + CSP estrita + gzip/cache | mesma origem (sem CORS), defesa em profundidade contra XSS | [0009](docs/adr/0009-nginx-proxy-csp.md) |
| Dependências do front | gráfico SVG próprio, validação sem lib, UI sem kit; 4 dependências de execução | *bundle* de 106 kB gzip e CSP sem exceções | [0010](docs/adr/0010-lightweight-frontend-dependencies.md) |

## Princípios → onde estão aplicados

| Princípio | Onde no código |
|---|---|
| **S** — Responsabilidade única | `booking.Service` só aplica regras de agenda; `dashboard.Service` só define indicadores; o repositório só agrega/persiste; `server.fail` é o **único** tradutor erro→HTTP; handlers só decodificam/chamam/codificam |
| **O** — Aberto/fechado | novo recurso = novo pacote + linhas em `routes()` + um `case` em `fail`; novo status/regra não reescreve o resto ([como adicionar](docs/ARCHITECTURE.md#8-como-adicionar-um-recurso-passo-a-passo)) |
| **L** — Substituição de Liskov | `memstore.*` e `postgres.*` satisfazem os mesmos contratos **comprovadamente**: `internal/repotest` roda nos dois |
| **I** — Segregação de interfaces | interfaces de 1–6 métodos declaradas por quem consome (`booking.Catalog` tem **1** método; `dashboard.Upcoming`, 1) |
| **D** — Inversão de dependência | o domínio define as interfaces e não importa `pgx` nem `net/http`; `app` liga as implementações |
| **YAGNI** | sem ORM, framework, cache, filas, multi-profissional, reagendamento, recorrência, pagamentos, down-migrations |
| **KISS** | erros são valores (`errors.Is/As`); máquina de estados em 3 linhas; uma tabela de rotas; a concorrência é resolvida por **uma** restrição do banco, sem locks |
| **DRY** | `decode`/`writeJSON`/`writeProblem` únicos; suíte de contrato única; a tabela de rotas alimenta o roteador **e** o teste da spec |

## Documentação

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — pacotes, modelo de dados, fluxo de agendamento, máquina de estados, mapa de erros, OpenAPI, integração com o front, como adicionar um recurso e **§9 Frontend** (estrutura, sequência de auth/refresh, estado de servidor, como adicionar uma tela).
- [docs/TESTING.md](docs/TESTING.md) — estratégia (backend e frontend), números medidos, gate de cobertura e exclusões.
- [docs/adr/](docs/adr) — decisões: [stdlib vs framework](docs/adr/0001-stdlib-vs-framework.md) ·
  [agenda sem sobreposição](docs/adr/0002-non-overlapping-agenda.md) · [snapshot de preço/duração](docs/adr/0003-price-duration-snapshot.md) ·
  [agregações do dashboard (com `EXPLAIN`)](docs/adr/0004-dashboard-aggregations.md) · [JWT + refresh, papéis, CORS](docs/adr/0005-auth-jwt-refresh.md) ·
  [estado de servidor (TanStack Query)](docs/adr/0006-server-state-tanstack-query.md) · [tipos gerados do OpenAPI](docs/adr/0007-generated-api-types.md) ·
  [armazenamento de tokens](docs/adr/0008-token-storage.md) · [nginx, proxy e CSP](docs/adr/0009-nginx-proxy-csp.md) · [dependências enxutas no front](docs/adr/0010-lightweight-frontend-dependencies.md).
- Swagger UI embutido: `swagger-ui-dist` 5.33.1 (Apache-2.0), com a licença em [`backend/internal/httpapi/swagger/LICENSE`](backend/internal/httpapi/swagger/LICENSE).

## Limitações conhecidas e próximos passos

- **É um exemplo:** sem TLS (termine no proxy), sem métricas/tracing/auditoria, sem recuperação de senha nem verificação de e-mail,
  sem LGPD/retenção de dados de clientes. Os segredos do compose são de desenvolvimento.
- **Um único prestador.** Multi-profissional é o próximo passo natural: `provider_id` + `btree_gist` + `EXCLUDE (provider_id WITH =, tstzrange WITH &&)`
  ([ADR 0002](docs/adr/0002-non-overlapping-agenda.md)). Não há **reagendamento** (cancele e agende de novo), recorrência, lista de espera, horário de funcionamento/feriados
  nem bloqueio de agenda; um agendamento pode ser marcado em qualquer horário futuro (inclusive de madrugada).
- **Moeda única, sem pagamentos:** o preço é só o valor de tabela; "receita realizada" é a soma dos `completed`, não um fluxo de caixa.
- **O access token não é revogável:** vale até expirar (15 min) mesmo após o logout; HS256 com segredo compartilhado. Veja o [ADR 0005](docs/adr/0005-auth-jwt-refresh.md).
- **Rate limit em memória, por processo**, com chave = IP do par TCP (`X-Forwarded-For` não é confiado): atrás de proxy/NAT — **inclusive no Docker, onde todos
  os clientes aparecem com o IP do gateway** — o limite passa a ser compartilhado.
- **`PATCH` sem controle de versão** (última escrita vence); paginação por **offset**; busca `q` por `strpos(lower(…))` (varredura; `pg_trgm` resolveria em tabelas grandes).
- **O dashboard lê direto das tabelas:** rápido para o volume de uma agenda (ver o `EXPLAIN` no [ADR 0004](docs/adr/0004-dashboard-aggregations.md)), mas sem pré-agregação;
  os planos foram medidos uma vez, manualmente — não há *benchmark* automatizado.
- **Migrations só "up", aplicadas na partida** (com `pg_advisory_lock`): conveniência de demo; em produção, rode como job separado.
- **O `make demo` insere o histórico por SQL** (psql no contêiner do banco), porque a API recusa agendar no passado por desenho; os números do
  dashboard são os mesmos que a API calcula, mas esse passo não passa pela API.
- As imagens (distroless/não-root no back, nginx *unprivileged* no front) **não foram escaneadas** por vulnerabilidades; o CI (workflow) **ainda não rodou no GitHub** — foi verificado localmente com os mesmos comandos.

**Frontend**

- **XSS lê o refresh token.** A API só fala `Bearer` (sem cookies), então o refresh token precisa ficar em `sessionStorage` (access em memória). Mitigado por CSP estrita, React sem `innerHTML` e
  rotação com detecção de reuso — **não eliminado**; a correção real é cookie `HttpOnly` (mudança no backend). [ADR 0008](docs/adr/0008-token-storage.md). Consequência de produto: um login por aba, e sair numa aba não sai nas outras.
- **O limite de login por IP é compartilhado** atrás do nginx: todos os navegadores chegam à API com o IP do contêiner do nginx, então as 10 tentativas/min de `/v1/auth/*` valem para *todos* os usuários juntos
  (vimos isso rodando o E2E em sequência). A correção é o backend confiar em `X-Forwarded-For` de um proxy conhecido.
- **Fuso do negócio fixo no *build*** (`VITE_BUSINESS_TZ`): o contrato só devolve o fuso nas respostas do dashboard, não num endpoint de configuração; trocar o fuso exige reconstruir a imagem (e manter igual ao `BUSINESS_TZ` da API).
- **Não há `GET /v1/users`:** a tela de Usuários só cria contas e lista as criadas na sessão atual.
- **Selects com as primeiras 100 opções** (cliente/serviço no agendamento; `page_size` máximo da API): com milhares de clientes, é preciso um *combobox* com busca. Também não há visão de calendário/semana, reagendamento nem edição de agendamento (a API não os oferece).
- **"Concluir/Faltou" usam o relógio do navegador** para habilitar os botões; o servidor é a autoridade (`409 not_started` é tratado), mas um relógio muito errado mostra o botão no momento errado. O KPI "agendados" do período padrão tende a 0 (a API conta o que **começa** no período, que termina hoje).
- **Gráfico simples** (barras por dia, uma série por vez; sem zoom). **Sem i18n** (só pt-BR), sem PWA/modo offline, um único *chunk* de JS (sem divisão por rota). Os campos nativos de data/hora seguem o idioma do navegador.
- **Testado em Chromium** (Playwright e o navegador do Claude); Safari/Firefox não foram exercitados. Contraste conferido por inspeção nos dois temas, sem auditoria automática (axe).
- **Sem TLS nem `Strict-Transport-Security`** no nginx (termine HTTPS em um proxy/balanceador na frente).

## Licença

[MIT](LICENSE) © 2026 André Coura.

---

## English summary

**go-react-business-suite** is a *reference/sample* project (not a production product): a small-business suite — service catalog, customers,
appointments on a single agenda, and a results dashboard — with a **Go 1.24 + PostgreSQL 16** backend (stdlib `net/http`, `slog`, `pgx`, 4 direct
dependencies) and a **React 19 + TypeScript + Vite** frontend (TanStack Query, 4 runtime dependencies, 106 kB gzip). Highlights: appointments can **never overlap,
even under concurrency**, guaranteed by a PostgreSQL `EXCLUDE USING gist` constraint and proven by a real concurrency test (30 simultaneous
requests → 1 × 201 + 29 × 409); price and duration are **snapshotted** at booking time; JWT access + **rotating refresh tokens** (the SPA keeps the access token in memory,
shares one in-flight refresh among concurrent requests, and logs out on a refused/reused refresh); RFC 9457 errors; the frontend's types are **generated from the OpenAPI
contract** and checked in CI; nginx (non-root, strict CSP) serves the SPA and proxies `/api` (no CORS). 227 backend + 188 frontend tests, 5 Playwright E2E tests
against the full compose stack. Run `make up` (Docker only — no Go or Node on the host) and open <http://localhost:8096>. See [README.en.md](README.en.md).
