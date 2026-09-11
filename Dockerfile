FROM golang:latest
-v /var/run/docker.sock:/var/run/docker.sock
WORKDIR /app
COPY . .
RUN go build -o health_checker
CMD ["./health_checker"]
