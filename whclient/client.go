package whclient

import (
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/taskcluster/webhooktunnel/util"
	"github.com/taskcluster/webhooktunnel/wsmux"
)

type clientState int

const (
	stateRunning = iota
	stateBroken
	stateClosed
)

// Config ...
type Config struct {
	ID        string
	ProxyAddr string
	Token     string
	Retry     RetryConfig
	Logger    util.Logger
}

// Configurer is a function which can generate a Config object
// to be used by the client
type Configurer func() (Config, error)

// Client used to connect to a proxy instance and serve content
// over the proxy. Client implements net.Listener.
type Client struct {
	m          sync.Mutex
	id         string
	proxyAddr  string
	token      string
	url        atomic.Value
	retry      RetryConfig
	logger     util.Logger
	configurer Configurer
	session    *wsmux.Session
	state      clientState
	closed     chan struct{}
	acceptErr  net.Error
}

// New creates a new Client instance.
func New(configurer Configurer) (*Client, error) {
	config, err := configurer()
	if err != nil {
		return nil, err
	}

	cl := &Client{configurer: configurer}
	cl.setConfig(config)
	cl.closed = make(chan struct{}, 1)
	conn, url, err := cl.connectWithRetry()
	if err != nil {
		return nil, err
	}
	cl.url.Store(url)
	cl.session = wsmux.Client(conn, wsmux.Config{})
	return cl, nil
}

// URL returns the url at which the proxy serves the client's
// endpoints
func (c *Client) URL() string {
	return c.url.Load().(string)
}

// Accept is used to accept multiplexed streams from the
// proxy as net.Conn.
func (c *Client) Accept() (net.Conn, error) {
	select {
	case <-c.closed:
		return nil, ErrClientClosed
	default:
	}

	c.m.Lock()
	defer c.m.Unlock()
	if c.state == stateBroken || c.state == stateClosed {
		return nil, c.acceptErr
	}

	stream, err := c.session.Accept()
	if err != nil {
		c.state = stateBroken
		c.acceptErr = ErrClientReconnecting
		go c.reconnect()
		return nil, c.acceptErr
	}
	return stream, nil
}

// Addr returns the net.Addr of the underlying wsmux session
func (c *Client) Addr() net.Addr {
	return c.session.Addr()
}

// Close connection to the proxy
func (c *Client) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
		close(c.closed)
		go func() {
			c.m.Lock()
			defer c.m.Unlock()
			c.acceptErr = ErrClientClosed
			_ = c.session.Close()
		}()
	}
	return nil
}

func (c *Client) setConfig(config Config) {
	c.id = config.ID
	c.proxyAddr = util.MakeWsURL(config.ProxyAddr)
	c.token = config.Token

	c.retry = config.Retry.defaultValues()
	c.logger = config.Logger
	if c.logger == nil {
		c.logger = &util.NilLogger{}
	}
}

// connectWithRetry returns a websocket connection to the proxy
func (c *Client) connectWithRetry() (*websocket.Conn, string, error) {
	// if token is expired or not usable, get a new token from the authorizer
	if !util.IsTokenUsable(c.token) {
		config, err := c.configurer()
		if err != nil {
			return nil, "", ErrRetryFailed
		}
		c.setConfig(config)
	}

	// initial connection
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+c.token)
	header.Set("x-webhooktunnel-id", c.id)
	// initial attempt
	c.logger.Printf("trying to connect to %s", c.proxyAddr)
	conn, res, err := websocket.DefaultDialer.Dial(c.proxyAddr, header)
	if err != nil {
		if shouldRetry(res) {
			// retry connection and return result
			return c.retryConn()
		}
		c.logger.Printf("connection failed with error:%v, response:%v", err, res)
		if isAuthError(res) {
			return nil, "", ErrAuthFailed
		}
		return nil, "", ErrRetryFailed
	}
	c.logger.Printf("connected to %s ", c.proxyAddr)

	url := res.Header.Get("x-webhooktunnel-client-url")
	return conn, url, err
}

// retryConn is a utility function used by connectWithRetry to use exponential
// backoff to attempt reconnection
func (c *Client) retryConn() (*websocket.Conn, string, error) {
	// at this point, proxy should return proxyAddr like ws://register.domain.ext

	header := make(http.Header)
	header.Set("Authorization", "Bearer "+c.token)
	header.Set("x-webhooktunnel-id", c.id)

	currentDelay := c.retry.InitialDelay
	maxTimer := time.After(c.retry.MaxElapsedTime)
	backoff := time.After(currentDelay)

	for {
		select {
		case <-maxTimer:
			return nil, "", ErrRetryTimedOut
		case <-backoff:
			c.logger.Printf("trying to connect to %s", c.proxyAddr)
			conn, res, err := websocket.DefaultDialer.Dial(c.proxyAddr, header)
			if err == nil {
				url := res.Header.Get("x-webhooktunnel-client-url")
				return conn, url, nil
			}
			if !shouldRetry(res) {
				c.logger.Printf("connection to %s failed. could not connect", c.proxyAddr)
				return nil, "", ErrRetryFailed
			}
			c.logger.Printf("connection to %s failed. will retry", c.proxyAddr)

			currentDelay = c.retry.nextDelay(currentDelay)
			backoff = time.After(currentDelay)
		}
	}
}

// reconnect is used to repair broken connections
func (c *Client) reconnect() {
	c.m.Lock()
	defer c.m.Unlock()
	conn, url, err := c.connectWithRetry()
	if err != nil {
		// set error and return
		c.logger.Printf("unable to reconnect to %s", c.proxyAddr)
		c.acceptErr = ErrRetryFailed
		return
	}

	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}

	sessionConfig := wsmux.Config{
		// Log:              c.logger,
		StreamBufferSize: 4 * 1024,
	}
	c.session = wsmux.Client(conn, sessionConfig)
	c.url.Store(url)
	c.state = stateRunning
	c.logger.Printf("state: running")
	c.acceptErr = nil

}

// simple utility to check if client should retry connection
func shouldRetry(r *http.Response) bool {
	// may be that proxy is down for changing secrets and therefore unreachable
	if r == nil {
		return true
	}
	if r.StatusCode/100 == 4 || r.StatusCode/100 == 2 {
		return false
	}
	return true
}

func isAuthError(r *http.Response) bool {
	if r == nil {
		return false
	}
	return r.StatusCode == 401
}
