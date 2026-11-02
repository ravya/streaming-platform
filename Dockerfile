FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/insights-api ./cmd/insights-api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/insights-api /insights-api
COPY configs /configs
ENV TENANTS_FILE=/configs/tenants.json
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/insights-api"]
