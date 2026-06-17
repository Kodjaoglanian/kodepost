# KodePost

Gerador de sites estáticos (SSG) para blog pessoal, escrito em Go. Inspirado no fluxo do [Akita On Rails](https://akitaonrails.com).

Escreva em Markdown → `git push` → `git pull` na VPS → rode o binário → blog atualizado.

## Funcionalidades

- Posts em Markdown com front matter YAML (`title`, `date`, `tags`, `summary`, `featured`)
- Timeline cronológica na home, agrupada por mês/ano
- Sidebar de navegação nos posts individuais
- Página de tags dinâmica (`/tags/`)
- Páginas estáticas (`/sobre/`, etc.) via pasta `pages/`
- Toggle claro/escuro com persistência via `localStorage`
- Servidor HTTP embutido para preview local
- Imagens copiadas automaticamente de `assets/images/`

## Estrutura

```
kodepost/
├── posts/              # Artigos em .md
├── pages/              # Páginas estáticas em .md
├── templates/
│   └── layout.html     # Template único com Go templates
├── assets/
│   └── images/         # Imagens referenciadas nos posts
├── public/             # Saída gerada (não versionado)
├── main.go
├── go.mod
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

## Uso local

**Pré-requisito:** Go 1.23+

```bash
# Gerar o site
go run . 

# Gerar e servir em http://localhost:8765
go run . serve

# Porta customizada
go run . serve -port 3000
```

## Formato de um post

Crie um arquivo `.md` em `posts/`:

```yaml
---
title: "Título do Post"
date: "2026-06-17"
tags: ["Go", "Arquitetura"]
summary: "Resumo que aparece nos cards da home."
featured: true
---

Conteúdo em Markdown...
```

- `featured: true` → aparece nos Destaques da sidebar da home
- Se nenhum post for marcado, os 3 mais recentes são usados como destaques

## Páginas estáticas

Crie arquivos `.md` em `pages/` com front matter mínimo:

```yaml
---
title: "Sobre"
---

Conteúdo...
```

Acessível em `/sobre/`.

## Deploy com Docker

```bash
# Build e start
sudo docker compose up -d --build

# Logs
sudo docker compose logs -f

# Parar
sudo docker compose down
```

O site fica disponível em `http://localhost:8765`.

Para expor no seu domínio, configure um proxy reverso (nginx ou Caddy) apontando para a porta 8765.

### Exemplo com Caddy

```
seu-dominio.com {
    reverse_proxy localhost:8765
}
```

### Exemplo com nginx

```nginx
server {
    listen 80;
    server_name seu-dominio.com;
    location / {
        proxy_pass http://localhost:8765;
    }
}
```

## Makefile

```bash
make build   # compila o binário
make serve   # gera + serve localmente
make docker  # docker compose up --build
make clean   # remove binário e public/
```

## Segurança

- Imagem distroless (`gcr.io/distroless/static-debian12:nonroot`)
- Usuário não-root (`65532:65532`)
- Filesystem read-only no container
- Zero shell, zero gerenciador de pacotes na imagem de runtime
- HTML gerado durante o build — zero escrita em runtime

## Licença

MIT — Bruno Kodjaoglanian
