# syntax=docker/dockerfile:1

FROM node:22-alpine AS builder

WORKDIR /app
COPY web/package.json web/package-lock.json* ./
RUN npm ci 2>/dev/null || npm install

COPY web/ .
RUN npm run build

FROM nginx:1.27-alpine

COPY deploy/docker/web.nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
