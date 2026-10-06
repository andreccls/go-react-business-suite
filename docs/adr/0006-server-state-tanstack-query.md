# ADR 0006 — Estado de servidor com TanStack Query (sem Redux, sem store global)

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

Quase todo o estado do painel é **dado do servidor**: serviços, clientes, agendamentos, KPIs. Esse tipo de estado tem problemas
próprios — cache, *loading/erro* por consulta, requisições duplicadas, dados velhos depois de uma escrita, revalidação — que um
`useState` + `useEffect` por tela reimplementa (mal) em cada tela. O estado de **interface** (qual modal está aberto, o texto
digitado num filtro, a página atual) é pequeno e local.

## Decisão

- **TanStack Query v5** é a única camada de estado de servidor. Cada leitura é um hook em `src/api/hooks.ts`
  (`useServices`, `useAppointments`, `useSummary`…) com *query key* = recurso + parâmetros (`['appointments', {status, from, to, page}]`);
  as telas só consomem `{ data, isPending, isError, refetch }` e desenham os três estados (carregando, erro com "Tentar novamente", vazio).
- **Escritas são `useMutation` que invalidam o que podem afetar** (ex.: criar/cancelar agendamento invalida `appointments` e `dashboard`;
  editar cliente invalida `appointments`, porque o nome do cliente vem embutido). Nada de atualização otimista: o servidor é a
  autoridade (a agenda sem sobreposição e a máquina de estados vivem nele) — depois de um `409` a lista é **recarregada**
  (`onSettled`), não "consertada" no cliente.
- **Política de *retry*** (`lib/queryClient.ts`): nunca repete um `4xx` (a resposta é definitiva); repete uma vez falha de rede/`5xx`.
- **Estado de interface fica local** (`useState` na tela). Há exatamente **um** contexto React: a sessão (`AuthProvider`).
- Ao perder a sessão, `queryClient.clear()` descarta todo o cache — dados de um usuário nunca vazam para o próximo login.

## Alternativas consideradas

- **Redux Toolkit / RTK Query:** resolve o mesmo, com mais cerimônia (slices, store, middleware) para um app com ~6 telas. Descartado (YAGNI).
- **`useEffect` + `fetch` por tela:** sem cache nem invalidação; cada tela reimplementaria *loading/erro/stale*. Descartado.
- **SWR:** equivalente para leituras; o TanStack Query tem `useMutation`, invalidação por prefixo e `placeholderData` (listas paginadas sem "piscar") mais maduros.
- **Loaders/actions do React Router (data APIs):** acoplariam a busca de dados ao roteador; o modo biblioteca (`<Routes>`) basta.

## Consequências

- (+) Telas curtas e declarativas; invalidação em um lugar; testes simples (o cliente de teste usa `retry: false`).
- (+) Listas paginadas mantêm a página anterior na tela enquanto a próxima carrega (`keepPreviousData`).
- (−) Uma dependência a mais (parte dos 106 kB gzip do *bundle*). Aceito: o código equivalente escrito à mão seria maior e mais frágil.
- (−) A chave de cache inclui o objeto de parâmetros; mudar a forma dos parâmetros muda a chave — por isso os hooks ficam todos em um arquivo.
