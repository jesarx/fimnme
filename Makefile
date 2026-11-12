# FIMNME — tareas de desarrollo, compilación y despliegue.
# Software libre bajo la licencia GNU GPL v3.0; véase el archivo LICENSE.
#
# Uso habitual:
#   make dev                      servidor local en http://127.0.0.1:8080 (base en ./data)
#   make verificar                formato + go vet + pruebas
#   make build                    binarios estáticos para Debian 13 en bin/
#   make deploy VPS=usuario@host  compila, sube los binarios, migra la base y reinicia
#
# La compilación se hace en la máquina del desarrollador (Linux, macOS o
# Windows con make): Go compila de forma cruzada, por eso el VPS no necesita
# tener Go instalado.

GO      ?= go
# Plataforma de destino: Debian 13 (trixie). Para un VPS ARM: make build GOARCH=arm64
GOOS    ?= linux
GOARCH  ?= amd64
BIN     := bin

# Versión incrustada en el binario: etiqueta o commit de git. Es determinista
# para un mismo commit, condición de la compilación reproducible (RNF-19).
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo desarrollo)

# -trimpath y -buildid= eliminan rutas locales e identificadores variables:
# el mismo commit produce binarios idénticos bit a bit en cualquier máquina.
# -s -w omiten las tablas de depuración (binario más pequeño).
BANDERAS := -trimpath -buildvcs=false -ldflags '-s -w -buildid= -X main.version=$(VERSION)'

# Despliegue. VPS es el destino de ssh; SUDO se deja vacío si se entra como root:
#   make deploy VPS=root@203.0.113.10 SUDO=
VPS  ?=
SUDO ?= sudo

.PHONY: build dev test verificar formato limpiar reiniciar-datos deploy instalar-vps

## build: compila fimnme-web y fimnme-cli como ELF estáticos, sin CGO.
build:
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(BANDERAS) -o $(BIN)/fimnme-web ./cmd/fimnme-web
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build $(BANDERAS) -o $(BIN)/fimnme-cli ./cmd/fimnme-cli
	@sha256sum $(BIN)/fimnme-web $(BIN)/fimnme-cli 2>/dev/null || shasum -a 256 $(BIN)/fimnme-web $(BIN)/fimnme-cli

## dev: prepara la base local (./data) y arranca el servidor en modo desarrollo.
dev:
	$(GO) run ./cmd/fimnme-cli migrate
	$(GO) run ./cmd/fimnme-cli sembrar-demo
	$(GO) run ./cmd/fimnme-web --entorno desarrollo

## test: ejecuta las pruebas con cobertura. La cobertura de cada paquete cuenta
## también lo que ejercitan las pruebas de los demás (las de extremo a extremo
## de controllers/ recorren buena parte de models/).
test:
	@mkdir -p $(BIN)
	$(GO) test -coverpkg=./... -coverprofile=$(BIN)/cobertura.out ./...
	@$(GO) tool cover -func=$(BIN)/cobertura.out | tail -1

## formato: reescribe el código con el formato canónico de Go.
formato:
	gofmt -w .

## verificar: lo que debe pasar antes de entregar un paso.
verificar:
	@test -z "$$(gofmt -l .)" || (echo "Archivos sin formato (ejecute 'make formato'):"; gofmt -l .; exit 1)
	$(GO) vet ./...
	$(GO) test ./...

## limpiar: borra los binarios compilados (no toca la base local).
limpiar:
	rm -rf $(BIN)

## reiniciar-datos: BORRA la base local de desarrollo (./data) para empezar de cero.
reiniciar-datos:
	rm -rf data

## instalar-vps: preparación INICIAL del VPS (usuario, directorios, configuración). Una sola vez.
instalar-vps:
	@test -n "$(VPS)" || (echo "Falta el destino. Ejemplo: make instalar-vps VPS=usuario@host"; exit 1)
	scp deploy/instalar_vps.sh deploy/fimnme-web.service deploy/fimnme-web.env.ejemplo $(VPS):/tmp/
	ssh -t $(VPS) '$(SUDO) sh /tmp/instalar_vps.sh'

## deploy: compila, sube los binarios al VPS, migra la base y reinicia el servicio.
deploy: build
	@test -n "$(VPS)" || (echo "Falta el destino. Ejemplo: make deploy VPS=usuario@host"; exit 1)
	ssh $(VPS) 'rm -rf /tmp/fimnme-despliegue && mkdir -m 0700 /tmp/fimnme-despliegue'
	scp $(BIN)/fimnme-web $(BIN)/fimnme-cli deploy/fimnme-web.service deploy/actualizar_vps.sh $(VPS):/tmp/fimnme-despliegue/
	ssh -t $(VPS) '$(SUDO) sh /tmp/fimnme-despliegue/actualizar_vps.sh'
