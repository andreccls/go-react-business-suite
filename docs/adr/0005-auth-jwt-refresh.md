# ADR 0005 — Autenticação: JWT curto + refresh rotativo, papéis `admin`/`staff`, CORS e segredos

- **Status:** aceito
- **Data:** 2026-10-06

## Contexto

Um painel React (SPA) precisa autenticar usuários internos (recepção/gestão), distinguir quem pode remover dados, e conviver
com duas topologias: **dev** (SPA em `localhost:5173`, API em `localhost:8095` — origens diferentes) e **produção** (nginx
serve o SPA e faz proxy de `/api` para a API — mesma origem).

## Decisão

- **Senhas:** bcrypt (8–72 bytes; o bcrypt ignora o que passa de 72). Login com e-mail inexistente compara com um hash falso,
  para não revelar pelo tempo de resposta quais e-mails existem.
- **Access token:** JWT HS256, 15 min, claims `sub`, `role`, `iss`, `iat`, `exp`. A validação fixa o algoritmo, exige `exp` e `iss`
  (barra `alg: none` e confusão de algoritmo). Sem consulta ao banco por requisição.
- **Refresh token:** 32 bytes aleatórios, opaco, guardado só como SHA-256, **uso único**. `POST /v1/auth/refresh` consome e emite um par
  novo (rotação). Reapresentar um token já usado revoga **todos** os refresh tokens do usuário (reuso = provável roubo).
  O consumo é um único `UPDATE … WHERE used_at IS NULL … RETURNING`, atômico.
- **Papéis:** `staff` lê/escreve tudo do dia a dia (serviços, clientes, agendamentos, dashboard); `admin` também **remove**
  serviços e clientes e **cria usuários**. Não há cadastro público: o primeiro admin vem de `ADMIN_EMAIL`/`ADMIN_PASSWORD`
  (idempotente), os demais de `POST /v1/users` (admin). Agendamentos nunca são removidos — cancelam-se.
- **Rate limit:** *token bucket* em memória — por IP em `/v1/auth/*` (10/min) e por usuário nas demais rotas (120/min); `429` + `Retry-After`.
  O IP é o do par TCP (`X-Forwarded-For` não é confiado: seria forjável); atrás de proxy, todos dividem o IP do proxy.
- **Segredos:** `JWT_SECRET` só por variável de ambiente. Com `APP_ENV=release` (o **padrão**) a API **recusa subir** com segredo < 32
  caracteres, de baixa entropia ou parecido com placeholder (`dev-only`, `change-me`…); idem para `ADMIN_PASSWORD`. O `docker-compose.yml`
  seta `APP_ENV=development` explicitamente e a API loga um aviso.
- **CORS (`CORS_ALLOWED_ORIGINS`):** lista de origens (ou `*`); vazio = desligado. Responde o *preflight* com 204 **antes** da
  autenticação e do rate limit, expõe `X-Request-ID`/`Retry-After`/`Location`, **nunca** permite credenciais (só Bearer, sem cookies).
  Em **produção** o nginx faz proxy e CORS fica desligado (a documentação do OpenAPI/`/docs` usa caminhos relativos, funcionando sob `/api/`).
  Exemplo de proxy (etapa 2): `location /api/ { proxy_pass http://api:8080/; }`.

## Alternativas consideradas

- **Cookie de sessão `HttpOnly`:** resolve o armazenamento no navegador, mas exige CSRF e mesma origem/`SameSite`; para uma API
  consumida também por outros clientes, Bearer é mais direto. Trade-off do SPA (token em memória + refresh) documentado no front.
- **JWT longo (sem refresh):** não há como encerrar uma sessão roubada. **Provedor externo (OIDC):** fora de escopo do exemplo.

## Consequências

- (+) Roubo de refresh token é detectável; vazamento do banco não entrega tokens utilizáveis; subir inseguro por esquecimento é difícil.
- (−) O access token não é revogável (vale até expirar, 15 min) mesmo após *logout*; HS256 com segredo compartilhado (para outros
  serviços verificarem tokens, usar RS256/EdDSA); o rate limit é por processo (várias réplicas multiplicam o limite).
