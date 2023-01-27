FROM alpine
COPY chisel /app/
ENTRYPOINT ["/app/chisel"]
