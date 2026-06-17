.PHONY: build serve dev docker docker-run clean

# Build local do binário
build:
	GOWORK=off go build -o kodepost .

# Preview local (gera + serve)
serve:
	GOWORK=off go run . serve

# Docker: build da imagem
docker:
	docker compose build kodepost

# Docker: run em produção
docker-run:
	docker compose up -d kodepost

# Docker: run em modo dev (com volume mounts para posts/templates)
dev:
	docker compose --profile dev up --build kodepost-dev

# Limpar public/ e binário local
clean:
	rm -rf public/ kodepost
