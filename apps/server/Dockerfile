FROM --platform=$BUILDPLATFORM node:24-bookworm-slim AS web
WORKDIR /src
RUN corepack enable pnpm
COPY package.json pnpm-workspace.yaml pnpm-lock.yaml ./
COPY apps/web/package.json apps/web/package.json
RUN pnpm install --frozen-lockfile
COPY apps/web apps/web
RUN pnpm build:web

FROM golang:1.25-bookworm AS server
WORKDIR /src
COPY apps/server/go.mod apps/server/go.sum ./
RUN go mod download
COPY apps/server .
RUN CGO_ENABLED=0 go build -trimpath -o /server ./cmd/server

FROM python:3.12-slim-bookworm AS runtime
WORKDIR /app
COPY packages/beancount-engine /opt/beancount-engine
RUN pip install --no-cache-dir /opt/beancount-engine
ADD --chmod=0644 --checksum=sha256:f52c4d237bcb092cbf02a29373d3e59d95349c2828a91491818ae437ee220f74 https://files.pythonhosted.org/packages/eb/21/ed48e671a5e474c620762a7ea9c6b7f402f847d74dd2b73ceb5d2dec79a3/beancount-3.2.3.tar.gz /usr/share/beanframe/sources/beancount-3.2.3.tar.gz
RUN chmod 0755 /usr/share/beanframe /usr/share/beanframe/sources
COPY . /usr/share/beanframe/source
COPY --from=web /src/apps/web/build /app/web
COPY --chown=10001:10001 testdata/ledger /app/example
COPY --chown=10001:10001 testdata/preview /app/preview
ENV LISTEN_ADDR=:8080 \
    WEB_DIR=/app/web \
    LEDGER_PATH=/app/example/main.beancount
EXPOSE 8080
USER 10001:10001

FROM runtime AS final
COPY --from=server /server /usr/local/bin/beanframe
ENTRYPOINT ["beanframe"]
