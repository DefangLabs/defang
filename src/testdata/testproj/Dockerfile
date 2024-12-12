FROM alpine:latest AS testproj
ARG DNS
ENV DNS=$DNS
CMD ["sh", "-c", "while true; do nslookup ${DNS} ; sleep 10 ; done"]
