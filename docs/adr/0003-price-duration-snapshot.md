# ADR 0003 — Preço, duração e nome congelados no agendamento (snapshot)

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

O catálogo muda: o preço sobe, a duração é ajustada, o serviço é renomeado. Se um agendamento só guardasse `service_id`,
editar o catálogo reescreveria o passado: a **receita realizada** de meses atrás mudaria, e um horário já reservado poderia
passar a se sobrepor a outro (a duração mudou, o `ends_at` não).

## Decisão

Ao agendar, o `booking.Service` lê o serviço e **copia para o agendamento**: `service_name`, `duration_min` e `price_cents`.
O fim é derivado uma única vez: `ends_at = starts_at + duration_min`. Depois disso, o agendamento **nunca mais consulta o
catálogo** para valores — ele guarda `service_id` apenas como referência (chave estrangeira `ON DELETE RESTRICT`).

- A **receita** do dashboard é a soma de `appointments.price_cents` dos `completed` — nunca `services.price_cents`
  (ver [ADR 0004](0004-dashboard-aggregations.md)).
- Editar o serviço afeta só os agendamentos **futuros** a serem criados; testado em `booking_test.go`
  (`TestCreateSnapshotsServiceAndDerivesEnd`), na API (`TestBookingHappyPathAndSnapshot`) e no `make demo`
  (preço 5000 → 9900 → 5000, e o agendamento anterior permanece em 5000).
- Serviço **inativo** não pode ser agendado (`422 service_inactive`), mas continua existindo para o histórico. Excluir um
  serviço referenciado é recusado (`409 service_in_use`): o caminho recomendado é desativar.
- `customer_name` na resposta **não** é snapshot: vem do cadastro atual (um campo de leitura preenchido por JOIN).
  Corrigir o nome de um cliente deve refletir nos agendamentos dele.

## Alternativas consideradas

- **Referência viva ao catálogo (JOIN):** o menor modelo, mas reescreve o histórico e quebra a invariante da agenda. Descartado.
- **Versionar o catálogo (tabela `service_versions`):** correto e mais flexível, porém muito mais esquema e consultas para
  um ganho que o snapshot já entrega. Se for preciso auditar *todas* as versões, é a evolução natural.
- **Guardar o catálogo inteiro como JSON no agendamento:** menos explícito e sem tipos; as 3 colunas são consultáveis e indexáveis.

## Consequências

- (+) Receita e agenda são estáveis no tempo; consultas de agregação só leem a tabela de agendamentos (veja o plano de execução).
- (+) O front mostra o que foi contratado, mesmo que o catálogo tenha mudado.
- (−) Duplicação deliberada de 3 colunas; correções retroativas de preço exigem editar o agendamento (não há endpoint para isso — YAGNI).
