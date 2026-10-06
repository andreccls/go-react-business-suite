# syntax=docker/dockerfile:1

# --- build: install from the lockfile, typecheck, bundle ---
FROM node:22-alpine AS build
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-fund --no-audit
COPY . .
# The business time zone is baked in at build time (the contract only exposes it on dashboard responses).
ARG VITE_BUSINESS_TZ=America/Sao_Paulo
ENV VITE_BUSINESS_TZ=$VITE_BUSINESS_TZ
RUN npm run build

# --- runtime: unprivileged nginx (uid 101, port 8080) serving the SPA and proxying /api ---
FROM nginxinc/nginx-unprivileged:1.28-alpine
COPY nginx/security-headers.conf /etc/nginx/snippets/security-headers.conf
COPY nginx/default.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 8080
