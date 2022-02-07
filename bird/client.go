package bird

import (
	"bufio"
	"net"
	"strings"

	"github.com/pkg/errors"
)

type Client interface {
	Query(cmd string) ([]string, error)
	Close() error
}

type SocketClient struct {
	scanner *bufio.Scanner
	con     net.Conn
}

func NewSocketClient(socketPath string, bufferSize int) (*SocketClient, error) {
	con, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to bird socket")
	}

	scanner := bufio.NewScanner(con)
	c := &SocketClient{
		scanner: scanner,
		con:     con,
	}

	_, err = c.scanConnection()
	if err != nil {
		return nil, errors.Wrap(err, "error while scanning initial connection")
	}

	return c, nil
}

func (c *SocketClient) Query(cmd string) ([]string, error) {
	cmd = strings.Trim(cmd, "\n") + "\n"

	_, err := c.con.Write([]byte(cmd))
	if err != nil {
		return nil, errors.Wrap(err, "failed to query bird socket")
	}

	output, err := c.scanConnection()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read from bird socket")
	}

	return output, nil
}

func (c *SocketClient) Close() error {
	return c.con.Close()
}

func (c *SocketClient) scanConnection() ([]string, error) {
	lines := make([]string, 0)
	for {
		if !c.scanner.Scan() {
			return nil, errors.New("socket died while reading")
		}
		line := c.scanner.Text()
		lines = append(lines, line)
		if strings.HasPrefix(line, "0") {
			return lines, nil
		}
	}
}
