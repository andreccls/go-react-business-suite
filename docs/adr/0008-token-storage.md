# ADR 0008 — Onde guardar os tokens no navegador (e a limitação de XSS)

- **Status:** aceito, com limitação conhecida
- **Data:** 2026-10-06

## Contexto

A API autentica com `Authorization: Bearer` (access JWT de 15 min) e um **refresh token de uso único** com rotação; reapresentar um
refresh já usado revoga **todos** os refresh tokens do usuário ([ADR 0005](0005-auth-jwt-refresh.md)). A API **não** emite cookies —
então não há como usar cookie `HttpOnly`, a única opção que JavaScript injetado (XSS) não consegue ler. É preciso guardar os
tokens em algum lugar acessível ao JavaScript e documentar o que isso custa.

## Decisão

| Token | Onde | Por quê |
|---|---|---|
| **Access** (15 min) | **só em memória** (variável do módulo `auth/session.ts`) | nunca é escrito em disco/Storage; some ao recarregar a página, e é recuperado com o refresh |
| **Refresh** (7 dias) | **`sessionStorage`** | sobrevive a um *reload* da aba, morre ao fechá-la |

Por que `sessionStorage` e não `localStorage`: `localStorage` é compartilhado por todas as abas, e o refresh token é **de uso
único**. Duas abas renovando ao mesmo tempo (ou uma aba velha reapresentando o token que a outra acabou de gastar) pareceriam,
para o servidor, **roubo** — e derrubariam a sessão do usuário em todas as abas. Com `sessionStorage` cada aba tem a sua própria
sessão (um login por aba) e a corrida entre abas não existe. Custo aceito: abrir o app numa aba nova pede login; sair numa aba não sai nas outras.

Mecânica ([sequência em ARCHITECTURE §9.2](../ARCHITECTURE.md#92-autenticação-e-refresh-sequência)): o cliente anexa o access token; num `401` faz **um**
refresh — **compartilhado** por todas as requisições simultâneas (um só `POST /v1/auth/refresh` em voo; sem isso, N requisições
paralelas gastariam o mesmo token N vezes e disparariam a detecção de reuso) — e **repete a requisição uma vez**. Se o refresh for
recusado (`401`: expirado, já usado, revogado) a sessão é encerrada e o usuário volta ao login; falhas de rede/`5xx`/`429` no refresh
**não** encerram a sessão. O logout chama `POST /v1/auth/logout` (revoga o refresh no servidor) e limpa o armazenamento local.

## Limitação (honesta): XSS lê o refresh token

Qualquer script que rode na página — por XSS em código nosso ou numa dependência comprometida — pode ler o `sessionStorage` e
exfiltrar o refresh token (e, pior, usá-lo para obter access tokens pelos próximos 7 dias, até alguém revogar). Mitigações **aplicadas**:
React escapa tudo por padrão e o código **não usa `dangerouslySetInnerHTML`/`innerHTML`**; **CSP** `script-src 'self'` (sem inline,
sem CDN, sem `eval`) no nginx ([ADR 0009](0009-nginx-proxy-csp.md)); access token curto e só em memória; refresh rotativo com detecção de
reuso (um token roubado e usado revela-se na próxima renovação legítima); `X-Frame-Options`/`frame-ancestors` contra *clickjacking*.
**A correção de verdade** exige mudança no backend: emitir o refresh em cookie `HttpOnly; Secure; SameSite=Strict` (+ proteção CSRF),
mantendo o access token em memória. Está registrado como próximo passo; não foi feito porque a etapa 1 definiu o contrato Bearer.

## Alternativas consideradas

- **`localStorage` para o refresh:** sobrevive ao fechar a aba, mas tem a corrida entre abas descrita acima e o mesmo risco de XSS. Descartado.
- **Tudo em memória:** a melhor postura contra XSS persistente, mas cada *reload* exige login. Ruim demais para um painel de uso diário.
- **Cookie `HttpOnly` (BFF ou mudança na API):** a opção certa para produção; fora do escopo e do contrato atual.
- **Access token em `sessionStorage` também:** nada a ganhar; manter em memória custa uma ida ao refresh por *reload*.

## Consequências

- (+) Um roubo de disco/backup do navegador não entrega access token; a corrida de refresh entre abas não existe.
- (−) XSS bem-sucedido = sessão comprometida (limitação acima). (−) Um login por aba.
- Testes: `auth/session.test.ts` (o access token nunca vai ao Storage; Storage que lança erro), `api/client.test.ts` (refresh único para N
  requisições, reuso, falha de rede) e `auth/auth.test.tsx` (restaurar após reload, expiração no meio da sessão, refresh revogado → login).
