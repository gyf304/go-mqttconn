# mqttconn

A sane MQTT client for go based on `paho.mqtt.golang` that implements `net.PacketConn`
(and therefore `net.Conn`, `io.ReadWriteCloser`, `io.ReadWriter`)

# A quick peek
```
// connect to hivemq public broker with no password, with a default topic test
conn, _ := mqttconn.DialMQTT("mqtt://broker.hivemq.com/test")
// you can also do urls like "mqtt://username:password@server:port/topic"
// or you can just omit to topic like "mqtt://broker.hivemq.com", in which
// case Write and Read are not going to work before calling SetDefaultTopic
// and Subscribe

// publish to topic test
conn.Write([]byte("hello world!"))
// publish to another topic
conn.WriteTo([]byte("hello world!"), mqttconn.TopicAddr("anothertopic"))

// read conn
readBuffer := make([]byte, 4096)
n, addr, err := conn.ReadFrom(readBuffer)
// addr.String() is the topic from which the message is received.
// readBuffer[:n] is the received message
```
