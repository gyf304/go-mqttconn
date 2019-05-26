package mqttconn

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
	"github.com/pkg/errors"
)

// MQTTConn wraps around mqtt and provides net.PacketConn functionality
type MQTTConn struct {
	mqtt.Client

	defaultTopicSet  bool
	defaultTopic     string
	defaultQoS       int
	readDeadline     time.Time
	writeDeadline    time.Time
	readChan         chan mqtt.Message
}

// DialMQTT acts like DialUDP or DialTCP
// takes in a url like mqtt://username:password@server:port/topic
func DialMQTT(uri string) (conn *MQTTConn, err error) {
	// Parse uri
	parsedURL, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	mqttProtocol := "tcp"
	port := parsedURL.Port()
	portAppend := ""
	if port != "" {
		portAppend = ":" + port
	}
	switch parsedURL.Scheme {
	case "mqtt":
		mqttProtocol = "tcp"
		if parsedURL.Port() == "" {
			portAppend = ":1883"
		}
	case "mqtts":
		mqttProtocol = "ssl"
	}
	mqttURI := fmt.Sprintf("%s://%s%s", mqttProtocol, parsedURL.Host, portAppend)
	user := parsedURL.User

	// build options
	opts := mqtt.NewClientOptions()
	opts.AddBroker(mqttURI)
	id, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	opts.SetClientID(id.String())
	if user != nil {
		opts.SetUsername(user.Username())
		password, passwordSet := user.Password()
		if passwordSet {
			opts.SetPassword(password)
		}
	}
	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	err = token.Error()
	if err != nil {
		return nil, err
	}
	conn, err = CreateMQTTConn(client)
	if err == nil {
		if parsedURL.Path != "" {
			defaultTopic := strings.TrimPrefix(parsedURL.Path, "/")
			err = conn.Subscribe(defaultTopic, 0)
			conn.SetDefaultTopic(defaultTopic)
		}
	}
	return conn, err
}

// Subscribe subscribes to a topic
func (conn *MQTTConn) Subscribe(topic string, qos int) error {
	conn.Client.Subscribe(topic, byte(qos), func(client mqtt.Client, msg mqtt.Message) {
		conn.readChan <- msg
	})
	return nil
}

// SetDefaultTopic sets default topic of a MQTTConn, which Write uses
func (conn *MQTTConn) SetDefaultTopic(topic string) {
	conn.defaultTopic = topic
	conn.defaultTopicSet = true
}

// SetDefaultQoS sets default QOS of a MQTTConn, which Write uses
func (conn *MQTTConn) SetDefaultQoS(qos int) {
	conn.defaultQoS = qos
}

// CreateMQTTConn wraps around an existing mqtt.Client
func CreateMQTTConn(mqttClient mqtt.Client) (conn *MQTTConn, err error) {
	readChan := make(chan mqtt.Message, 2)
	return &MQTTConn{
		Client: mqttClient,
		readChan:   readChan,
	}, nil
}

// Write implements net.PacketConn.Write
func (conn *MQTTConn) Write(p []byte) (n int, err error) {
	return conn.WriteTo(p, TopicAddr(conn.defaultTopic))
}

// WriteTo implements net.PacketConn.WriteTo
func (conn *MQTTConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if addr.Network() != TopicAddr("").Network() {
		return 0, errors.New("unexpected net.Addr.Network() value")
	}
	token := conn.Client.Publish(addr.String(), byte(conn.defaultQoS), false, b)
	if conn.writeDeadline.IsZero() {
		token.Wait()
	} else {
		waitTime := conn.writeDeadline.Sub(time.Now())
		if waitTime <= 0 {
			return 0, &mqttError{true, errors.New("publish timed out")}
		}
		token.WaitTimeout(waitTime)
	}
	err := token.Error()
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// Read implements net.PacketConn.Read
func (conn *MQTTConn) Read(p []byte) (n int, err error) {
	n, _, err = conn.ReadFrom(p)
	return n, err
}

// ReadFrom implements net.PacketConn.ReadFrom
func (conn *MQTTConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	var timeout <-chan time.Time
	if conn.readDeadline.IsZero() {
		timeout = make(chan time.Time)
	} else {
		waitTime := conn.readDeadline.Sub(time.Now())
		if waitTime <= 0 {
			return 0, nil, &mqttError{true, errors.New("read timed out")}
		}
		timeout = time.After(waitTime)
	}
	
	select {
	case msg := <-conn.readChan:
		copiedCount := copy(p, msg.Payload())
		return copiedCount, TopicAddr(msg.Topic()), nil
	case <-timeout:
		return 0, nil, &mqttError{true, errors.New("read timed out")}
	}
}

// SetDeadline implements net.PacketConn.SetDeadline
func (conn *MQTTConn) SetDeadline(t time.Time) error {
	conn.readDeadline = t
	conn.writeDeadline = t
	return nil
}

// SetReadDeadline implements net.PacketConn.SetReadDeadline
func (conn *MQTTConn) SetReadDeadline(t time.Time) error {
	conn.readDeadline = t
	return nil
}

// SetWriteDeadline implements net.PacketConn.SetWriteDeadline
func (conn *MQTTConn) SetWriteDeadline(t time.Time) error {
	conn.writeDeadline = t
	return nil
}

// LocalAddr implements net.PacketConn.LocalAddr
func (conn *MQTTConn) LocalAddr() net.Addr {
	return TopicAddr("")
}

// RemoteAddr implements net.PacketConn.RemoteAddr
func (conn *MQTTConn) RemoteAddr() net.Addr {
	return TopicAddr(conn.defaultTopic)
}

// Close implements net.PacketConn.Close
func (conn *MQTTConn) Close() error {
	close(conn.readChan)
	conn.Client.Disconnect(100)
	return nil
}

// TopicAddr is topic name conforming to the net.Addr interface
type TopicAddr string

// Network implements net.Addr.Network()
func (addr TopicAddr) Network() string {
	return "mqttTopic"
}

// String implements net.Addr.String()
func (addr TopicAddr) String() string {
	return string(addr)
}

// mqttError implements error
type mqttError struct {
	isTimeout bool
	err       error
}

// Timeout determines if a error is a timeout error
func (err *mqttError) Timeout() bool {
	return err.isTimeout
}

func (err *mqttError) Error() string {
	return err.err.Error()
}
