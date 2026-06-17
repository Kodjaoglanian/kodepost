---
title: "Como construí este blog com Go"
date: "2026-06-17"
tags: ["Go", "Arquitetura", "Docker"]
summary: "Ao invés de usar um gerador de sites pronto, escrevi o meu próprio em Go. Aqui está o que aprendi sobre simplicidade, performance e por que às vezes a melhor ferramenta é aquela que você mesmo faz."
featured: true
---

## O problema com geradores existentes

Quando decidi criar meu blog pessoal, a primeira reação foi instalar Hugo, Jekyll ou Eleventy. São ótimas ferramentas — mas carregam uma complexidade que não precisava.

Queria algo simples: escrevo em Markdown, dou `git push`, e o blog atualiza. Sem runtime de Node.js, sem dependências de Ruby, sem configurações obscuras. Só arquivos HTML estáticos servidos de forma direta.

A decisão foi: **escrever meu próprio gerador de sites em Go**.

## A estrutura é deliberadamente simples

```
kodepost/
├── posts/          # Markdown com front matter YAML
├── pages/          # Páginas estáticas (Sobre, etc.)
├── templates/      # Um único layout.html com Go templates
├── public/         # Saída — arquivos HTML gerados
└── main.go         # ~500 linhas. Tudo.
```

Sem framework. Sem plugins. Sem magia. O binário compilado faz tudo: lê os posts, converte Markdown para HTML, injeta nos templates e escreve os arquivos na pasta `public/`.

## Go é perfeito para isso

Três razões:

**1. Biblioteca padrão suficiente.** O pacote `html/template` resolve renderização de forma segura por padrão (escape automático de HTML). O `net/http` serve os arquivos localmente sem nenhuma dependência extra.

**2. Compilação para binário estático.** Com `CGO_ENABLED=0`, o resultado é um único binário que roda em qualquer Linux. Zero runtime necessário.

**3. Rápido para escrever e rápido para executar.** O gerador compila todos os posts em menos de 100ms mesmo com dezenas de artigos.

As únicas dependências externas são duas:

```go
import (
    "github.com/yuin/goldmark"  // Markdown → HTML
    "gopkg.in/yaml.v3"          // Front matter
)
```

## Front matter com YAML nativo

Cada post começa com metadados delimitados por `---`:

```yaml
---
title: "Título do Post"
date: "2025-06-17"
tags: ["Go", "Blog"]
summary: "Resumo que aparece nos cards da home."
featured: true
---
```

O parser é direto: divide o arquivo pela primeira ocorrência de `---`, faz `yaml.Unmarshal` do bloco do meio e passa o restante para o Goldmark converter.

## Templates com lógica mínima

Um único arquivo `layout.html` serve todas as páginas. A lógica de ramificação é feita com Go templates:

```html
{{if .IsHome}}
  <!-- timeline cronológica -->
{{else if .IsPage}}
  <!-- página estática -->
{{else}}
  <!-- post individual com sidebar -->
{{end}}
```

O dado injetado é uma struct `PageData` simples com os campos que cada view precisa. Nada de mágica — se você sabe o que está no template, sabe o que está na struct.

## Docker com imagem distroless

O deploy usa um Dockerfile multi-stage:

1. **Stage builder** — compila o binário e gera o HTML estático durante o próprio build
2. **Stage runtime** — copia apenas o binário e a pasta `public/` para uma imagem `gcr.io/distroless/static-debian12:nonroot`

O resultado é uma imagem com menos de 15MB, sem shell, sem gerenciador de pacotes, rodando como usuário não-root. O filesystem é montado como read-only no Docker Compose.

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o kodepost .
RUN ./kodepost  # gera public/ durante o build

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /build/kodepost ./
COPY --from=builder /build/public ./public
EXPOSE 8765
CMD ["serve", "-port", "8765"]
```

## O que aprendi

> Complexidade não resolvida é complexidade transferida para quem usa.

Ferramentas genéricas resolvem 100 casos de uso — mas o seu caso específico pode ser resolvido com 5% desse esforço, de forma mais direta e mais fácil de manter.

O gerador tem ~500 linhas de Go. Qualquer um pode ler, entender e modificar em uma tarde. Esse é o ponto.

Se você está considerando criar seu próprio blog, considere também criar a ferramenta. O processo de construir é tão valioso quanto o resultado.
