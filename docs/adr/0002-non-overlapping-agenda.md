# ADR 0002 — Agenda sem sobreposição, garantida pelo banco

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

Há **uma única agenda** (um prestador): dois agendamentos não podem ocupar o mesmo horário. A regra precisa valer com
requisições simultâneas — o caso clássico de *check-then-insert*: duas requisições leem "horário livre" ao mesmo tempo e
ambas inserem.

## Decisão

A regra é uma **restrição de exclusão do PostgreSQL**, não código da aplicação (migração `0002_core.sql`):

```sql
CONSTRAINT appointments_no_overlap
  EXCLUDE USING gist (tstzrange(starts_at, ends_at, '[)') WITH &&)
  WHERE (status IN ('scheduled', 'completed'))
```

- **Intervalo semiaberto `[início, fim)`:** um agendamento que termina às 10:30 e outro que começa às 10:30 **não** conflitam.
- **Quais status ocupam a agenda:** `scheduled` e `completed`. `cancelled` e `no_show` **liberam** o horário (a máquina de
  estados só sai de `scheduled`, então nenhuma transição pode "reocupar" um horário e violar a restrição depois do INSERT).
- **Atomicidade:** o PostgreSQL verifica a restrição com o índice GiST dentro do próprio `INSERT`; um segundo `INSERT`
  concorrente espera o primeiro confirmar/abortar e então falha com `SQLSTATE 23P01`. O repositório traduz isso para
  `booking.ErrSlotTaken` → HTTP **409** `slot_unavailable`. Não há *lock* explícito, transação serializável nem retry.
- **Contrato do `booking.Repository`:** "`Create` é atômico quanto à sobreposição". A implementação em memória (`memstore`)
  cumpre o mesmo contrato com um mutex que cobre verificação + inserção, e **a mesma suíte de testes roda nas duas**.
- `btree_gist` **não** é necessária: o índice é só sobre um `tstzrange`. Ela passaria a ser necessária ao incluir uma coluna
  escalar na exclusão (ex.: `provider_id WITH =`) — exatamente o próximo passo (multi-profissional, abaixo).

## Como foi provado

1. `internal/repotest` → `Appointments` (roda em memória **e** no PostgreSQL real):
   - 25 *goroutines* criam o **mesmo horário** ao mesmo tempo: **exatamente 1** sucesso e 24 `ErrSlotTaken`; 1 linha gravada.
   - 24 criações simultâneas com inícios escalonados de 15 em 15 min (todas se sobrepõem): ao final, **nenhum par** de linhas gravadas se sobrepõe.
   - tabela de bordas: idêntico, começa dentro, termina dentro, contém, contido, encostado antes/depois, outro dia.
2. `internal/app` → `TestConcurrentBookingsOnePerSlot`: 30 `POST /v1/appointments` simultâneos pela pilha completa (HTTP → serviços → pgx →
   PostgreSQL): **1 × 201 e 29 × 409**.
3. `internal/postgres` → `TestExclusionConstraintGuardsRawInserts`: um `INSERT` cru, sem passar pelo repositório, também é recusado
   — a garantia está no esquema.
4. Plano de execução da verificação (EXPLAIN no [ADR 0004](0004-dashboard-aggregations.md)): o `INSERT` consulta o índice GiST
   (`Bitmap Index Scan on appointments_no_overlap`), não varre a tabela.

## Alternativas consideradas

| Alternativa | Por que não |
|---|---|
| `SELECT … FOR UPDATE` / checar e inserir na mesma transação | Em `READ COMMITTED` **não** impede o *phantom*: não há linha para travar quando o horário está livre. |
| Transação `SERIALIZABLE` + retry | Funciona, mas empurra para a aplicação um laço de retry e erros `40001` que o banco resolve sozinho com a restrição. |
| `pg_advisory_lock` por agenda | Serializa **todas** as criações (mesmo em dias diferentes) e a garantia depende de todo código lembrar do lock. |
| Verificação só em Go (mutex) | Só vale para 1 processo; quebra com 2 réplicas ou qualquer escrita fora da API. |

## Consequências

- (+) A invariante vive no esquema: vale para qualquer cliente, réplica ou script. Menos código, nenhum *lock* a gerenciar.
- (+) Cancelar libera o horário sem lógica extra.
- (−) Acopla a regra ao PostgreSQL (aceito: o projeto é PostgreSQL-first; o `memstore` existe só para testes).
- (−) O erro do banco não diz *qual* agendamento conflita; a API responde apenas "horário indisponível". Para mostrar o conflitante
  seria preciso uma consulta extra após o 23P01.
- **Próximo passo — multi-profissional:** adicionar `provider_id`, `CREATE EXTENSION btree_gist` e
  `EXCLUDE USING gist (provider_id WITH =, tstzrange(starts_at, ends_at, '[)') WITH &&) WHERE (…)`. A API e o domínio quase não mudam.
