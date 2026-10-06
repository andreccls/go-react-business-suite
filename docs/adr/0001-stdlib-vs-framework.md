# ADR 0001 — `net/http` da biblioteca padrão em vez de framework web

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

O backend precisa de roteamento com parâmetros de caminho (`/v1/appointments/{id}/status`), middleware (request ID, log,
CORS, timeout, autenticação), JSON, erros padronizados e documentação OpenAPI. As opções realistas em Go são: a stdlib
(`net/http` + `ServeMux` desde o Go 1.22), um roteador leve (chi) ou um framework (Gin, Echo, Fiber).

## Decisão

Usar **só a stdlib** para HTTP (`net/http`, `ServeMux` com padrões `"METHOD /caminho/{id}"`, `encoding/json`, `log/slog`) e
**4 dependências diretas** no total, cada uma com um motivo que a stdlib não cobre:

| Dependência | Para quê |
|---|---|
| `jackc/pgx/v5` | driver/pool PostgreSQL (a stdlib só tem `database/sql`, sem tipos nativos do Postgres nem `pgconn.PgError`) |
| `golang-jwt/jwt/v5` | assinatura/validação de JWT (criptografia própria seria um risco, não uma economia) |
| `golang.org/x/crypto/bcrypt` | hash de senha |
| `google/uuid` | UUIDs |

Padrões que a stdlib não dá de graça foram escritos à mão, curtos e testados: a tabela de rotas (`server.routes()`), os
middlewares (`func(http.Handler) http.Handler`), o *token bucket* (`internal/ratelimit`, ~60 linhas), o CORS (~30 linhas) e o
formato de erro RFC 9457.

## Alternativas consideradas

- **chi / Gin / Echo:** economizam pouco código aqui (as rotas são ~25), trazem uma segunda forma de escrever handlers
  (`gin.Context`) e acoplam o código de negócio ao framework. O `ServeMux` do Go 1.22+ já resolve método + parâmetro de caminho.
- **ORM (GORM) / `sqlc`:** as consultas importantes do projeto — a restrição de exclusão e as agregações do dashboard — são SQL
  que se quer **ler**, não gerar. Com ~25 consultas, `pgx` direto é mais claro e mais fácil de revisar.
- **Framework de validação por tags:** a validação é regra de negócio (ex.: "duração entre 5 e 480 min", "início no futuro");
  vive no `Service`, em Go, testada em tabela, e devolve erros por campo (`validation.Errors`).

## Consequências

- (+) Binário pequeno, imagem *distroless*, poucas atualizações de segurança para acompanhar, nenhum "mágico" a depurar.
- (+) O contrato de rotas é uma estrutura de dados (`routes()`), usada pelo roteador **e** pelo teste que compara com o
  `openapi.json` — impossível divergirem em silêncio.
- (−) Código de infraestrutura a mais (middlewares, `decode`, `writeProblem`) que um framework traria pronto: algumas centenas de linhas, todas cobertas por testes.
- (−) O `ServeMux` não distingue `405` de `404` (ambos viram `route_not_found`).
