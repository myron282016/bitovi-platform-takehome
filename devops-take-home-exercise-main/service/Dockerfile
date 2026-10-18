FROM golang:1.22

WORKDIR /app
COPY . .
RUN go build -o /order-processor .

EXPOSE 8080
CMD ["/order-processor"]
