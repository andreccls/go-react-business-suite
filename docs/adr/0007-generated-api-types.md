# ADR 0007 — Tipos do frontend gerados do `openapi.json` (e verificados no CI)

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

O contrato da API é o `backend/internal/httpapi/openapi.json`, e o backend já tem testes que o mantêm **idêntico ao roteador**
(ver [ARCHITECTURE §6](../ARCHITECTURE.md#6-contrato-openapi-que-nunca-diverge)). Se o frontend declarasse os tipos à mão
(`interface Service {…}`), eles divergiriam em silêncio na primeira mudança de campo — e o erro só apareceria em produção, como
`undefined` na tela.

## Decisão

- `frontend/src/api/schema.d.ts` é **gerado** por `openapi-typescript` a partir do arquivo do backend (`npm run gen:api` / `make frontend-types`)
  e **versionado** (o `docker build` do frontend não precisa enxergar a pasta `backend/`).
- `src/api/types.ts` só dá apelidos legíveis (`Service = components['schemas']['Service']`) e deriva os tipos de *query string*
  dos `operations` (`ServiceQuery`, `AppointmentQuery`…). Nenhum tipo de domínio é escrito à mão.
- `src/api/endpoints.ts` tem **uma função tipada por operação** do contrato, sobre um cliente HTTP fino (`src/api/client.ts`).
  Não usamos um cliente gerado (`openapi-fetch`, SDK gerado): o cliente próprio é ~120 linhas e precisa de uma regra que nenhum
  gerador conhece — a **fila única de refresh** ([ADR 0008](0008-token-storage.md)).
- **Verificação:** `npm run check:api` regenera para um arquivo temporário e dá `diff` com o versionado; roda em `make lint` e
  no job `frontend` do CI. Se alguém mudar o `openapi.json` sem regenerar, o build quebra e a mensagem é o próprio diff.
- **O *mock* dos testes herda o contrato:** os dados do MSW (`src/test/mockApi.ts`) são tipados com os mesmos tipos gerados;
  remover/renomear um campo na spec quebra a compilação dos testes — antes de qualquer teste rodar.

## Alternativas consideradas

- **Tipos à mão:** simples hoje, divergem amanhã. Descartado.
- **Gerar o cliente inteiro (openapi-fetch, orval, openapi-generator):** mais código gerado e mais dependências; não resolve o refresh e esconde a política de erros (RFC 9457 → `ApiError`). Descartado.
- **Validar respostas em tempo de execução (zod a partir da spec):** pegaria um backend fora do contrato, mas o backend já valida cada resposta contra a spec nos próprios testes. Custo (peso + CPU) sem ganho aqui; reavaliar se houver outro produtor da API.

## Consequências

- (+) Mudou o contrato → `tsc` aponta cada tela afetada. A fonte da verdade continua única.
- (−) Os tipos descrevem o que a spec **promete**, não o que o servidor devolveu num caso de bug; erros `5xx` sem JSON viram `ApiError('unexpected_response')`.
- (−) `schema.d.ts` é um arquivo gerado grande no repositório (revisado só pelo `diff` do CI).
