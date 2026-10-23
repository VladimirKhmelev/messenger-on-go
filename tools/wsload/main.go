// Command wsload loads ws-gateway over WebSocket, so that the calls it makes
// to chat-service can be compared by transport: run one gateway over gRPC
// and the other over QUIC (WS_GATEWAY_1_CHAT_TRANSPORT and
// WS_GATEWAY_2_CHAT_TRANSPORT in docker-compose.yml) and load both alike.
//
// wsload connects users to every gateway and sends them a mix of requests at
// a rate that rises in steps, with Poisson arrivals, without waiting for the
// answers. It prints the latency of the answers per step, gateway and
// request, counted from the moment a request was due, so that queueing in
// the gateway shows too.
//
// The users are made up: wsload signs their tokens with the JWT secret of
// the stack and gives each a chat with itself, the one chat chat-service
// creates without another user.
//
// Usage:
//
//	wsload -gateways grpc=ws://localhost:8080/ws,quic=ws://localhost:8081/ws -steps 50,200,500
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"
)

// sendBudget is how many messages a minute wsload lets a user send, out of
// the 30 chat-service allows, with some to spare.
const sendBudget = 25

type gateway struct{ name, url string }

type config struct {
	gateways []gateway
	chatURL  string
	secret   string
	users    int       // per gateway
	rates    []float64 // requests per second over all the gateways, per step
	step     time.Duration
	warmup   time.Duration
	drain    time.Duration
	mix      mix
	seed     uint64
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("wsload: ")
	c, err := parseFlags(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rep, err := run(ctx, c)
	if err != nil {
		log.Fatal(err)
	}
	rep.print(os.Stdout)
}

func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("wsload", flag.ContinueOnError)
	gateways := fs.String("gateways", "ws-gateway-1=ws://localhost:8080/ws,ws-gateway-2=ws://localhost:8081/ws",
		"the gateways to load, `[name=]url,...`; the names label the report")
	chatURL := fs.String("chat", "http://localhost:8083", "HTTP address of chat-service, where the users get their chats")
	secret := fs.String("jwt-secret", "dev-secret", "JWT_SECRET of the stack, to sign the tokens of the users")
	users := fs.Int("users", 200, "users per gateway, each on a connection of its own")
	steps := fs.String("steps", "50,200,500", "requests per second over all the gateways, a `rate,...` per step")
	step := fs.Duration("step", time.Minute, "how long every step lasts")
	warmup := fs.Duration("warmup", 10*time.Second, "how long to load at the rate of the first step before measuring")
	drain := fs.Duration("drain", 10*time.Second, "how long to wait for the answers after the last step; later ones count as lost")
	mixFlag := fs.String("mix", "get_presence=45,get_read_status=45,send_message=10", "shares of the requests, `request=weight,...`")
	seed := fs.Uint64("seed", 1, "seed of the arrival times and of the choice of requests")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() > 0 {
		return config{}, fmt.Errorf("unexpected arguments %q", fs.Args())
	}

	c := config{chatURL: strings.TrimSuffix(*chatURL, "/"), secret: *secret, users: *users,
		step: *step, warmup: *warmup, drain: *drain, seed: *seed}
	for i, g := range strings.Split(*gateways, ",") {
		name, addr, ok := strings.Cut(g, "=")
		if !ok {
			name, addr = "gateway-"+strconv.Itoa(i+1), g
		}
		if u, err := url.Parse(addr); err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
			return config{}, fmt.Errorf("gateway %q: want a ws:// or wss:// URL", g)
		}
		c.gateways = append(c.gateways, gateway{name: name, url: addr})
	}
	for _, r := range strings.Split(*steps, ",") {
		rate, err := strconv.ParseFloat(r, 64)
		if err != nil || rate <= 0 {
			return config{}, fmt.Errorf("step %q: want a positive rate", r)
		}
		c.rates = append(c.rates, rate)
	}
	var err error
	if c.mix, err = parseMix(*mixFlag); err != nil {
		return config{}, err
	}
	switch {
	case c.users < 1:
		return config{}, errors.New("-users must be at least 1")
	case c.step <= 0 || c.warmup < 0 || c.drain < 0:
		return config{}, errors.New("-step must be positive, -warmup and -drain not negative")
	}
	// Every user of a gateway sends in turn, so each sends an equal share.
	for _, rate := range c.rates {
		if perUser := rate / float64(len(c.gateways)) * c.mix[sendMessage] * 60 / float64(c.users); perUser > sendBudget {
			return config{}, fmt.Errorf("at %g rps every user would send %.0f messages a minute, over the %d wsload allows to stay under chat-service's limit of 30; add -users or lower the share of send_message",
				rate, perUser, sendBudget)
		}
	}
	return c, nil
}
