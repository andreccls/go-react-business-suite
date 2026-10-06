# ADR 0004 — Agregações do dashboard: definições, SQL e índices

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

O painel precisa de KPIs do período, série diária, top serviços e próximos agendamentos, rápidos e **conferíveis à mão**.

## Decisão

### Definições (uma só fonte: `internal/dashboard`)

| Indicador | Definição |
|---|---|
| Período | `from`/`to` = datas `YYYY-MM-DD`, **inclusivas**, no fuso do negócio (`BUSINESS_TZ`). Padrão: últimos 30 dias até hoje. Máximo 366 dias; `from ≤ to`. Vira o intervalo semiaberto `[from 00:00, to+1d 00:00)` em instantes. |
| Contagem por status | agendamentos cujo **início** cai no período, por status (`scheduled`, `completed`, `cancelled`, `no_show`) |
| `revenue_cents` | **receita realizada** = soma de `price_cents` (snapshot) dos `completed` |
| `average_ticket_cents` | `revenue_cents / completed`, arredondado ao centavo (0 sem `completed`) |
| `cancellation_rate`, `no_show_rate` | `cancelled / total` e `no_show / total`, sobre **todos** os agendamentos do período; razão 0..1 com 4 casas (0 sem agendamentos) |
| `new_customers` | clientes com `created_at` no período |
| Série diária | um ponto por **dia de calendário** do período, **zero-preenchido** (o gráfico nunca tem buracos) |
| Top serviços | por receita realizada, depois nº de `completed`, depois nº de agendamentos, depois nome (desempate determinístico) |
| Próximos | `scheduled` com início ≥ agora, do mais próximo ao mais distante |

O **repositório só agrega** (`GROUP BY`); as definições (receita, razões, ticket, zero-preenchimento) ficam em Go puro,
testáveis em tabela e com 100% de cobertura. O mesmo contrato roda no fake em memória e no PostgreSQL
(`repotest.Dashboard`), com um conjunto de dados calculado à mão — inclusive um agendamento às 22:30 locais que cai no dia seguinte em UTC.

### Fuso horário

Instantes são `timestamptz` (UTC). O "dia" é decidido no banco com `(starts_at AT TIME ZONE $tz)::date`; o filtro continua sendo
um intervalo de **instantes** sobre `starts_at`, então o índice é usado. A imagem *distroless* não tem base de fusos: o binário
embute `time/tzdata`.

### Consultas e índices

Cada consulta é **uma varredura por faixa em `starts_at`** + `GROUP BY`; o custo cresce com o tamanho do **período**, não da tabela.

```sql
CREATE INDEX appointments_starts_at_idx ON appointments (starts_at) INCLUDE (status, price_cents, service_id);  -- KPIs, série, ranking
CREATE INDEX appointments_scheduled_idx ON appointments (starts_at) WHERE status = 'scheduled';                 -- próximos
CREATE INDEX appointments_customer_idx  ON appointments (customer_id, starts_at);                               -- histórico do cliente + FK
CREATE INDEX appointments_service_idx   ON appointments (service_id);                                           -- FK (delete de serviço)
CREATE INDEX customers_created_at_idx   ON customers (created_at);                                              -- novos clientes
```

### Prova com `EXPLAIN (ANALYZE, BUFFERS)`

Massa: **250.000 agendamentos** (2024-01 → 2026-05, 6 serviços, 2.000 clientes), `VACUUM ANALYZE`, PostgreSQL 16, janela de 30 dias (8.640 linhas).
Reproduzível: carregar as migrações num banco descartável, inserir a massa com `generate_series` e rodar os `EXPLAIN`.

| Consulta | Plano | Buffers | Tempo de execução |
|---|---|---|---|
| KPIs por status — **com** o índice | `Index Only Scan using appointments_starts_at_idx` (`Heap Fetches: 0`) → `HashAggregate` | 66 | **1,9 ms** |
| a mesma, **sem** os índices | `Parallel Seq Scan on appointments` (descarta 80.453 linhas por worker) | 4.327 | 31,6 ms |
| Série diária (30 dias) | `Index Only Scan` → `Sort` → `GroupAggregate` | 66 | 7,3 ms |
| Top serviços (30 dias, `LIMIT 5`) | `Index Only Scan` + `Hash Join` com `services` (6 linhas) | 73 | 2,0 ms |
| Próximos (`LIMIT 5`) | `Index Scan using appointments_scheduled_idx` → `Incremental Sort` | 22 | 0,14 ms |
| Verificação da exclusão no `INSERT` | `Bitmap Index Scan on appointments_no_overlap` (GiST) | 5 | 0,08 ms |

Trecho do plano principal:

```
HashAggregate (actual rows=4 loops=1)
  Group Key: status
  ->  Index Only Scan using appointments_starts_at_idx on appointments (actual rows=8640 loops=1)
        Index Cond: ((starts_at >= '2026-04-01 00:00:00+00') AND (starts_at < '2026-05-01 00:00:00+00'))
        Heap Fetches: 0
Execution Time: 1.865 ms        -- sem índice: Parallel Seq Scan, Buffers: shared hit=4327, 31.630 ms
```

(`INCLUDE (status, price_cents, service_id)` é o que permite o *index-only scan*: a consulta não toca a tabela. Isso depende do
*visibility map* — o autovacuum o mantém; logo após uma carga grande sem `VACUUM`, o plano pode voltar a visitar o *heap*.)

## Alternativas consideradas

- **Tabela de resumo / visão materializada:** só compensa com dezenas de milhões de linhas; adiciona invalidação e atraso. YAGNI.
- **Calcular tudo em Go lendo as linhas:** move milhares de linhas pela rede para somar. O `GROUP BY` devolve ≤ 31 linhas.
- **Um único endpoint `/dashboard`:** acopla cargas distintas; quatro endpoints permitem ao front carregar e falhar por widget.

## Consequências

- (+) Números conferíveis à mão (o `make demo` recalcula e compara), plano com índice comprovado, período limitado a 366 dias.
- (−) O índice `INCLUDE` custa espaço e escrita; aceitável para um volume de agenda. A busca textual `q` usa `strpos(lower(...))`
  (varredura sequencial): `pg_trgm` seria o próximo passo para tabelas grandes.
