package mqttconn

import (
	"testing"
	"net"
	"github.com/google/uuid"
)

func TestBasic(t *testing.T) {
	// compile time test: mqtt.Conn conforms to net.PacketConn
	var pktConn net.PacketConn
	pktConn = &MQTTConn{}
	pktConn.LocalAddr()
	// test connection to public broker, and test simple pub sub
	uid, err := uuid.NewRandom()
	if err != nil {
		panic("not able to generate uuid")
	}
	topic := "test/" + uid.String()
	conn, err := DialMQTT("mqtt://broker.hivemq.com/" + topic)
	if err != nil {
		t.Error(err)
		return
	}
	if conn == nil {
		t.Error("conn is nil")
		return
	}
	expectedRead := "mqttconn test"
	conn.Write([]byte(expectedRead))
	readBuf := make([]byte, 64)
	n, addr, err := conn.ReadFrom(readBuf)
	if err != nil {
		t.Error(err)
		return
	}
	if addr.String() != topic {
		t.Error("expected addr", topic, "got", addr.String())
		return
	}
	actualRead := string(readBuf[:n])
	if actualRead != expectedRead {
		t.Error("expected read", expectedRead, "actual read", actualRead)
		return
	}
}
