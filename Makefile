all:client server
	
client:
	go build -o ./bin/ ./p2pclient/

server:
	go build -o ./bin/ ./p2pserver/

