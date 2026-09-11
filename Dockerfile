FROM golang:latest
WORKDIR /app
COPY . .
RUN go build -o health_checker
CMD ["./health_checker"]