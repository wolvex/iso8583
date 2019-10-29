package iso8583

import (
	"bufio"
	"fmt"
	"math/rand"
	"net"
	"strconv"
	"time"

	log "github.com/Sirupsen/logrus"
)

type Client interface {
	SignOn() error
	TearDown()
	AddTicker()
	Disconnect() error
	EchoTest()
	Send(msg *IsoMsg, res chan *IsoMsg) error
	GetStan() int
}

type IsoClient struct {
	ID          string
	Address     string
	Port        int
	Timeout     int
	Link        net.Conn
	Packager    *StringPackager
	LastSend    time.Time
	LastReceive time.Time
	SignedOn    bool
	Stan        int
	Ticker      int
	Outgoing    int64
	Incoming    int64
}

type Payload struct {
	Request   *IsoMsg
	Response  chan *IsoMsg
	Timestamp time.Time
}

func NewClient(address string, port, timeout int, spec map[int]ElementSpec) (client *IsoClient, err error) {
	defer func() {
		if err != nil && client != nil {
			if client.Link != nil {
				client.Link.Close()
			}
			client = nil
		}
	}()

	client = &IsoClient{
		ID:      fmt.Sprintf("iso@%d", rand.Int()),
		Address: address,
		Port:    port,
		Timeout: timeout,
		Ticker:  0,
	}

	url := fmt.Sprintf("%s:%d", address, port)

	log.WithField("server", url).Info("Connecting to ISO8583 server")

	//open connection to eload
	if client.Link, err = net.Dial("tcp", url); err != nil {
		return
	}

	//initialize i/o buffer stream and packager
	input := bufio.NewReader(client.Link)
	output := bufio.NewWriter(client.Link)
	client.Packager = NewStringPackager(spec, input, output)

	//initialize variables
	client.Stan, _ = strconv.Atoi(time.Now().Format("040500"))

	return
}

func (c *IsoClient) GetID() string {
	return c.ID
}

func (c *IsoClient) AddTicker() {
	c.Ticker = c.Ticker + 1
	if c.Ticker >= 3 {
		c.TearDown()
	}
	return
}

func (c *IsoClient) IsValid() bool {
	return c.SignedOn && c.LastReceive.Add(60*time.Second).After(time.Now())
}

func (c *IsoClient) SignOn() (err error) {

	msg := NewIsoMsg()
	msg.SetMessageType("0800")
	msg.SetBit(7, time.Now().Format("0102150405"))
	msg.SetBit(11, fmt.Sprintf("%d", 1))
	msg.SetBit(70, "001")

	log.WithField("iso8583", msg.Dump()).Info("Sign-on request")

	//set read timeout
	if err = c.Link.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}

	if err = c.Packager.Send(msg); err != nil {
		return err
	}

	var res []byte
	if res, err = c.Packager.Read(); err != nil {
		return err
	}

	if msg, err = c.Packager.Unpack(res); err != nil {
		return err
	}

	log.WithField("iso8583", msg.Dump()).Info("Sign-on response")

	var rc int
	if rc, err = msg.GetRespCode(); err != nil {
		return
	}

	if rc != ISO_ERR_SUCCESS {
		err = fmt.Errorf("Received failed response for sign-on")
		return
	}

	log.Info("Sign-On Successful")

	c.LastSend = time.Now()
	c.LastReceive = time.Now()
	c.SignedOn = true

	return
}

func (c *IsoClient) TearDown() {
	c.SignedOn = false
}

func (c *IsoClient) Disconnect() error {
	return c.Link.Close()
}

func (c *IsoClient) EchoTest(queue map[string]*Payload) {

	go func() (err error) {
		defer func() {
			if err != nil {
				log.WithField("error", err).Error("Exception caught")
				c.SignedOn = false
			}
		}()

		msg := NewIsoMsg()
		msg.SetMessageType("0800")
		msg.SetBit(7, time.Now().Format("0102150405"))
		msg.SetBit(70, "301")

		log.WithField("iso8583", msg.Dump()).Debug("Echo request")
		inbox := make(chan *IsoMsg)
		if err = c.Send(msg, queue, inbox); err != nil {
			return
		}

		select {
		case msg := <-inbox:
			log.WithField("iso8583", msg.Dump()).Debug("Echo response")
			var rc int
			if rc, err = msg.GetRespCode(); err != nil {
				return err

			} else if rc != ISO_ERR_SUCCESS {
				return fmt.Errorf("Received failed response for echo-test")

			}
			break

		case <-time.After(5 * time.Second):
			return fmt.Errorf("Timeout detected !!!")
		}

		return nil
	}()
}

func (c *IsoClient) Send(msg *IsoMsg, queue map[string]*Payload, res chan *IsoMsg) error {
	msg.SetBit(11, fmt.Sprintf("%d", c.GetStan()))

	key := messageKey(msg)

	payload := &Payload{
		Request:   msg,
		Response:  res,
		Timestamp: time.Now(),
	}

	if queue != nil {
		log.WithFields(log.Fields{
			"messageKey": key,
			"iso8583":    msg.Dump(),
		}).Debug("Push request to master queue")

		queue[key] = payload
	}

	log.WithFields(log.Fields{
		"messageKey": key,
		"iso8583":    msg.Dump(),
	}).Info("Submit request")

	if err := c.Packager.Send(msg); err != nil {
		//sending has failed, remove from queue if any
		if queue != nil {
			delete(queue, key)
		}
		return err
	}

	c.LastSend = time.Now()
	c.Outgoing++

	return nil
}

func (c *IsoClient) Receive(queue map[string]*Payload) (err error) {
	//set read timeout
	if err = c.Link.SetReadDeadline(time.Now().Add(time.Duration(c.Timeout) * time.Millisecond)); err != nil {
		return err
	}

	var msg []byte
	if msg, err = c.Packager.Read(); err != nil {
		return err
	}

	var res *IsoMsg
	if res, err = c.Packager.Unpack(msg); err != nil {
		return err
	}

	c.LastReceive = time.Now()
	c.Incoming++

	key := messageKey(res)

	log.WithFields(log.Fields{
		"messageKey": key,
		"iso8583":    res.Dump(),
	}).Info("Received response")

	if queue == nil {
		return
	}

	log.WithFields(log.Fields{
		"messageKey": key,
		"iso8583":    res.Dump(),
	}).Debug("Finding request payload in queue")

	if payload, ok := queue[key]; ok {
		//push response back to caller
		log.WithFields(log.Fields{
			"messageKey": key,
			"iso8583":    res.Dump(),
		}).Debug("Pushing response to callback queue")

		payload.Response <- res

		delete(queue, key)
	}

	return
}

func (c *IsoClient) GetStan() int {
	if c.Stan >= 999999 {
		c.Stan = 1
	} else {
		c.Stan++
	}
	return c.Stan
}

func messageKey(msg *IsoMsg) string {
	mty := msg.GetMessageType()[0:2]
	if mty == "08" {
		return fmt.Sprintf("%s-%010s-%06s-%06s", mty, msg.GetBit(7), msg.GetBit(11), msg.GetBit(32))
	} else {
		return fmt.Sprintf("%s-%s-%010s-%06s-%06s", mty, msg.GetBit(3)[0:2], msg.GetBit(7), msg.GetBit(11), msg.GetBit(32))
	}
}

func isTimeout(err error) bool {
	if err, ok := err.(net.Error); ok && err.Timeout() {
		return true
	} else {
		return false
	}
}

/*
func (c *IsoClient) PushResponse(msg []byte) error {
	//parse message
	if res, err := c.Packager.Unpack(msg); err != nil {
		return err

	} else {
		key := messageKey(res)

		log.WithFields(log.Fields{
			"messageKey": key,
			"iso8583":    res.Dump(),
		}).Info("Received response")

		if payload, ok := c.Table[key]; ok {
			//push response to caller
			payload.Response <- res
		}

	}

	return nil
}
*/
