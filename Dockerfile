# 构建阶段
FROM golang:1.21-alpine AS build
WORKDIR /src
COPY . .
RUN go build -o /ledgerflow .

# 运行阶段
FROM alpine:latest
WORKDIR /app
COPY --from=build /ledgerflow /app/ledgerflow
ENV LEDGERFLOW_HOME=/data
VOLUME ["/data"]
ENTRYPOINT ["/app/ledgerflow"]
CMD ["help"]
