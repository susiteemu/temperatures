FROM amd64/alpine:latest

WORKDIR /app

COPY ["ruuvitag-httpserver", ".env", "config.yml", "./"]

ENTRYPOINT ["./ruuvitag-httpserver"]
