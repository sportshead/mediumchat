package main

import (
	"bufio"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

type Client struct {
	Id   int
	Name string
	Conn net.Conn
}

var clients = make(map[int]Client)
var names = make(map[string]struct{})

type Message struct {
	Sender  int
	Message string
}

const ServerPrefix = "server!"
const Motd = `%[1]s Welcome to MediumChat.
%[1]s You are %s.
%[1]s Commands:
%[1]s   - /nick [nick]: Change or reset your nickname
%[1]s   - /disconnect: Disconnect from the server
`

func main() {
	addr := flag.String("addr", ":4242", "address to listen on")
	flag.Parse()

	go func() {
		os.Exit(runServer(*addr))
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		broadcastMessage(Message{
			Sender:  0,
			Message: fmt.Sprintf("%s %s\n", ServerPrefix, scanner.Text()),
		})
	}
	if err := scanner.Err(); err != nil {
		slog.Error("failed to read from stdin", slogTag("read_stdin_failed"), slogError(err))
	}
}

func runServer(addr string) int {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("failed to create listener", slogTag("listen_failed"), slogError(err))
		return 1
	}
	slog.Info("server listening", slogTag("listening"), slog.String("addr", addr))

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		l.Close()
	}()

	nextClientId := 0
	for {
		conn, err := l.Accept()
		if err != nil {
			slog.Error("failed to accept connection", slogTag("accept_failed"), slogError(err))
			return 1
		}
		defer conn.Close()

		nextClientId++
		client := Client{
			Id:   nextClientId,
			Name: fmt.Sprintf("user:%d", nextClientId),
			Conn: conn,
		}
		clients[client.Id] = client
		go handleClient(client)
	}
}

func broadcastMessage(msg Message) {
	slog.Info("message received",
		slogTag("new_msg"),
		slog.Int("from", msg.Sender),
		slog.String("message", string(msg.Message)))

	for id, client := range clients {
		if id != msg.Sender {
			writeClient(msg.Message, client)
		}
	}
}

func handleClient(client Client) {
	logger := slog.With(slog.Int("client", client.Id))
	logger.Info("client connected", slogTag("client_connected"))
	writeClient(fmt.Sprintf(Motd, ServerPrefix, client.Name), client)

	broadcastMessage(Message{
		Sender:  client.Id,
		Message: fmt.Sprintf("%s %s joined.\n", ServerPrefix, client.Name),
	})

	scanner := bufio.NewScanner(client.Conn)
	for scanner.Scan() {
		input := scanner.Text()
		switch {
		case strings.HasPrefix(input, "/nick"):
			var newName string
			if len(input) < 7 {
				newName = fmt.Sprintf("user:%d", client.Id)
			} else {
				newName = input[6:]
				if strings.HasPrefix(newName, "server") || (strings.HasPrefix(newName, "user:") && newName != fmt.Sprintf("user:%d", client.Id)) {
					writeClient(fmt.Sprintf("%s Your new nickname, %s, is forbidden.\n", ServerPrefix, newName), client)
					break
				}
			}
			if _, ok := names[newName]; ok {
				writeClient(fmt.Sprintf("%s Your new nickname, %s, is currently in use.\n", ServerPrefix, newName), client)
				break
			}
			delete(names, client.Name)
			names[newName] = struct{}{}
			broadcastMessage(Message{
				Sender:  0,
				Message: fmt.Sprintf("%s %s changed their nickname to %s.\n", ServerPrefix, client.Name, newName),
			})
			client.Name = newName
		case strings.HasPrefix(input, "/disconnect"):
			writeClient("Goodbye!", client)
			client.Conn.Close()
			broadcastMessage(Message{
				Sender:  client.Id,
				Message: fmt.Sprintf("%s %s disconnected.\n", ServerPrefix, client.Name),
			})
		default:
			broadcastMessage(Message{
				Sender:  client.Id,
				Message: fmt.Sprintf("%s> %s\n", client.Name, input),
			})
		}
	}
	delete(clients, client.Id)
	if err := scanner.Err(); err != nil {
		logger.Error("failed to read from client", slogTag("read_client_failed"), slogError(err))
	}
}

func writeClient(msg string, client Client) {
	_, err := client.Conn.Write([]byte(msg))
	if err != nil {
		slog.Error("failed to write to client", slogTag("write_client_failed"), slogError(err), slog.Int("client", client.Id))
	}
}
