# ADR 0010 — Poucas dependências no frontend: gráfico em SVG próprio, validação e UI sem bibliotecas

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

O front é um painel pequeno de um estúdio. Cada dependência é peso no *bundle*, superfície de ataque (a CSP e o armazenamento de token
do [ADR 0008](0008-token-storage.md) dependem de o código de terceiros ser pouco) e algo a manter. As escolhas abaixo seguem YAGNI.

## Decisão

- **Gráfico: SVG próprio** (`components/DailyChart.tsx`, ~80 linhas) em vez de Recharts (dezenas de kB gzip, com dependências próprias) ou uPlot (leve, mas ainda uma dependência com CSS próprio). O app tem **um** gráfico
  (barras por dia). O SVG próprio custa **0 kB**, não injeta `<style>` (compatível com a CSP) e é acessível por construção:
  `role="img"` com resumo textual, `<title>` em cada barra e **a mesma série numa tabela real** (`<details>`). Eixo com 4 passos redondos
  (`axisTop`/`niceCeil`, testados). *Teto conhecido:* sem zoom/tooltip rico/múltiplas séries — se isso for pedido, trocar por uPlot.
- **Formulários: `useState` + validadores puros** (`lib/validation.ts`) em vez de react-hook-form + zod (duas dependências). São 5 formulários de 2–5 campos;
  as regras espelham os limites do contrato (nome 2–120, duração 5–480, preço ≤ R$ 100.000…) e o `422` da API é mesclado por campo
  (`fieldErrorsFrom`) — **a API continua sendo a autoridade**, o cliente só adianta o feedback. Dinheiro é digitado em reais e convertido
  em centavos **por texto** (`parseMoneyToCents`), nunca por `parseFloat`.
- **Roteamento: `react-router` em modo biblioteca** (sem *data APIs*); **sem UI kit** (CSS próprio com *custom properties*, claro/escuro por
  `prefers-color-scheme`, componentes acessíveis próprios: `Modal` com foco preso e retorno, `Field` ligando rótulo/dica/erro); **datas com `Intl`**
  (sem moment/date-fns): `lib/datetime.ts` converte *wall clock* do fuso do negócio ⇄ instante UTC, certo também na virada de horário de verão (testado).
- Dependências de **execução**: `react`, `react-dom`, `react-router`, `@tanstack/react-query` — **4**. *Bundle*: **~345 kB (106 kB gzip)** de JS e 2,7 kB gzip de CSS.

## Alternativas consideradas

- **Recharts / uPlot / Chart.js:** resolvem mais do que precisamos; o ganho (tooltips, zoom) não justifica o peso hoje.
- **react-hook-form + zod:** compensa com muitos formulários grandes ou esquemas compartilhados com o servidor; aqui, não.
- **Material UI / Radix / shadcn:** acelerariam telas novas, mas trariam estilos injetados (CSP) e centenas de kB para um app pequeno.
- **Divisão do *bundle* por rota (`React.lazy`):** não feita — um único *chunk* de 106 kB gzip carrega rápido o bastante; adicionar quando passar de ~250 kB.

## Consequências

- (+) *Bundle* pequeno, CSP estrita sem exceções, pouco a atualizar/auditar. (−) Mais código nosso para manter (modal, campo, gráfico) — por isso testado a ~100%.
- (−) Selects de cliente/serviço no agendamento carregam as primeiras **100** opções (limite de `page_size`); com milhares de clientes, trocar por um *combobox* com busca.
