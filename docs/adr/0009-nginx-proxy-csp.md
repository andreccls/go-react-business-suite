# ADR 0009 — nginx não-root serve o SPA, faz proxy de `/api` e aplica a CSP

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

Em produção o navegador precisa de **uma origem só**: assim não há CORS (a API fica com `CORS_ALLOWED_ORIGINS` vazio), o cookie/CSRF não
entra na conversa e a CSP pode ser `connect-src 'self'`. Além de servir os arquivos estáticos, o servidor precisa lidar com rotas
do lado do cliente (`/agenda` recarregada), cache de *assets* com *hash*, compressão e cabeçalhos de segurança.

## Decisão

- **Imagem em dois estágios** (`frontend/Dockerfile`): `node:22-alpine` instala do *lockfile* (`npm ci`), checa tipos e gera o *bundle*;
  `nginxinc/nginx-unprivileged` (usuário `nginx`, uid 101, porta **8080**, sem *capabilities* extras) só recebe `dist/`. Imagem final ~82 MB, sem Node.
- **`location /api/ { proxy_pass http://api:8080/; }`** — a barra final remove o prefixo `/api`. A API continua não sabendo do prefixo, e o
  Swagger UI embutido (que carrega a spec por caminho relativo) funciona em `/api/docs/`. A mesma convenção vale no `vite dev`
  (proxy do Vite), então **dev e produção usam o mesmo caminho** (`/api`) e CORS não é necessário em nenhum dos dois.
- **SPA:** `try_files $uri $uri/ /index.html` para rotas do cliente; `index.html` com `Cache-Control: no-cache`; `/assets/*`
  (nomes com *hash*) com `public, max-age=31536000, immutable` e **404 de verdade** quando falta um arquivo (não devolve o `index.html`).
- **CSP:** `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none';
  base-uri 'self'; form-action 'self'; frame-ancestors 'none'`. Funciona porque o app **não tem `<script>`/`<style>` inline, nem CDN, nem fontes
  externas** (fontes do sistema; Vite emite CSS/JS em arquivos) e o React define `style` pelo CSSOM, que `style-src 'self'` permite.
  Além disso: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Permissions-Policy`, `Cross-Origin-Opener-Policy`.
  Um `include` por *location* — o nginx **descarta** os `add_header` herdados assim que um *location* declara o seu.
- **gzip** para texto (o JS cai de ~345 kB para ~106 kB). `/healthz` próprio do nginx (usado pelo *healthcheck* do compose).
- A resposta do `/api/` leva só os cabeçalhos genéricos + `Cache-Control: no-store`: a CSP do SPA não é aplicada ao Swagger UI da API.

## Alternativas consideradas

- **Servir o SPA pela própria API Go (`embed`):** menos uma peça, mas mistura o ciclo de *deploy* do front com o do back e perde cache/gzip/CSP configuráveis. Descartado.
- **Caddy/Traefik:** também servem; nginx é o padrão que mais gente reconhece e a imagem *unprivileged* oficial existe. Equivalente.
- **CDN/hospedagem estática separada + CORS:** válida em produção real, mas reintroduz CORS e uma segunda origem; fora do escopo do exemplo.
- **CSP com `'unsafe-inline'` por conveniência:** anularia a principal defesa em profundidade contra XSS ([ADR 0008](0008-token-storage.md)). Descartado.

## Consequências

- (+) Mesma origem: sem CORS, sem *preflight*, CSP estrita. Processo sem root. Rotas profundas e *reload* funcionam.
- (−) Atrás do nginx **todo cliente chega à API com o IP do contêiner do nginx**; o *rate limit* por IP da API (`/v1/auth/*`: 10/min) vira
  **um limite compartilhado por todos os usuários** (já era uma limitação documentada do ADR 0005; aqui ela se materializa). A correção é
  o backend confiar em `X-Forwarded-For` de um proxy conhecido — nginx já envia o cabeçalho.
- (−) Sem TLS: termine HTTPS em um balanceador/proxy na frente (e então acrescente `Strict-Transport-Security`).
- (−) A CSP é estrita: qualquer biblioteca futura que injete `<style>` inline vai quebrar até ser ajustada.
