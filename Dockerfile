# Multi-stage build para máxima segurança e mínimo tamanho

# ─── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /build

# Cache de dependências
COPY go.mod go.sum ./
RUN go mod download

# Copiar código-fonte
COPY . .

# Compilar binário estático (sem CGO, linked estaticamente)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o kodepost .

# Gerar o site estático durante o build (para não precisar escrever no runtime)
RUN ./kodepost

# ─── Stage 2: Runtime ────────────────────────────────────────────────────────
# Distroless: zero pacotes desnecessários, mínima superfície de ataque
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Copiar apenas os arquivos estáticos gerados + binário
COPY --from=builder /build/kodepost ./
COPY --from=builder /build/public ./public

# O usuário nonroot (65532:65532) já vem na imagem distroless
# Expor a porta do servidor
EXPOSE 8765

# Comando padrão: gerar + servir
ENTRYPOINT ["./kodepost"]
CMD ["serve", "-port", "8765"]
