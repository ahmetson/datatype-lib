package message

import (
	"fmt"
	"time"

	"github.com/ahmetson/datatype-lib/data_type/key_value"
	"github.com/google/uuid"
)

// Stack keeps the parameters of the message in the service.
type Stack struct {
	RequestTime    uint64 `json:"request_time"`
	ReplyTime      uint64 `json:"reply_time,omitempty"`
	Command        string `json:"command"`
	ServiceUrl     string `json:"service_url"`
	ServerName     string `json:"server_name"`
	ServerInstance string `json:"server_instance"`
}

// DefaultMessage returns a message for parsing request and parsing reply.
func DefaultMessage() *Operations {
	return &Operations{
		Name:       "default",
		NewReq:     NewReq,
		NewReply:   NewRep,
		EmptyReq:   NewEmptyReq,
		EmptyReply: NewEmptyReply,
	}
}

func NewEmptyReq() RequestInterface {
	return &Request{}
}

// NewReq from the zeromq messages
func NewReq(messages []string) (RequestInterface, error) {
	msg := JoinMessages(messages)

	data, err := key_value.NewFromString(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to convert message string %s to key-value: %v", msg, err)
	}

	var request Request
	err = data.Interface(&request)
	if err != nil {
		return nil, fmt.Errorf("failed to convert key-value %v to intermediate interface: %v", data, err)
	}

	// verify that data is not nil
	_, err = request.Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to validate: %w", err)
	}

	if MultiPart(messages) {
		request.conId = messages[0]
	}

	return &request, nil
}

// Request message sent by Client socket and accepted by ControllerCategory socket.
type Request struct {
	Uuid       string             `json:"uuid,omitempty"`
	Trace      []*Stack           `json:"traces,omitempty"`
	Command    string             `json:"command"`
	Parameters key_value.KeyValue `json:"parameters"`
	publicKey  string
	conId      string // This one is used between sockets, generated by sockets
}

// CommandName returns a command name
func (request *Request) CommandName() string {
	return request.Command
}

// RouteParameters returns a command name
func (request *Request) RouteParameters() key_value.KeyValue {
	return request.Parameters
}

// ConId returns a connection id for each sending session.
func (request *Request) ConId() string {
	return request.conId
}

func (request *Request) SetConId(conId string) {
	request.conId = conId
}

func (request *Request) Traces() []*Stack {
	return request.Trace
}

// IsFirst returns true if the request has no trace,
//
// For example, if the proxy inserts it.
func (request *Request) IsFirst() bool {
	return len(request.Trace) == 0
}

// SyncTrace is if the reply has more stacks, the request is updated with it.
func (request *Request) SyncTrace(reply ReplyInterface) {
	repTraceLen := len(reply.Traces())
	reqTraceLen := len(request.Traces())

	if repTraceLen > reqTraceLen {
		request.Trace = append(request.Trace, reply.Traces()[reqTraceLen:]...)
	}
}

func (request *Request) AddRequestStack(serviceUrl string, serverName string, serverInstance string) {
	stack := &Stack{
		RequestTime:    uint64(time.Now().UnixMicro()),
		ReplyTime:      0,
		Command:        request.Command,
		ServiceUrl:     serviceUrl,
		ServerName:     serverName,
		ServerInstance: serverInstance,
	}

	request.Trace = append(request.Trace, stack)
}

// Bytes convert the message to the sequence of bytes
func (request *Request) Bytes() ([]byte, error) {
	err := ValidCommand(request.Command)
	if err != nil {
		return nil, fmt.Errorf("failed to validate command: %w", err)
	}

	kv, err := key_value.NewFromInterface(request)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize Request to key-value: %v", err)
	}

	bytes, err := kv.Bytes()
	if err != nil {
		return nil, fmt.Errorf("kv.Bytes: %w", err)
	}

	return bytes, nil
}

// SetPublicKey For security; Work in Progress.
func (request *Request) SetPublicKey(publicKey string) {
	request.publicKey = publicKey
}

// PublicKey For security; Work in Progress.
func (request *Request) PublicKey() string {
	return request.publicKey
}

// JoinMessages the message
func (request *Request) String() string {
	bytes, err := request.Bytes()
	if err != nil {
		return ""
	}

	return string(bytes)
}

func (request *Request) ZmqEnvelope() ([]string, error) {
	bytes, err := request.Bytes()
	if err != nil {
		return nil, fmt.Errorf("request.ZmqEnvelope: %w", err)
	}
	str := string(bytes)

	if len(request.conId) > 0 {
		return []string{request.conId, "", str}, nil
	}

	return []string{"", str}, nil
}

func (request *Request) SetUuid() {
	id := uuid.New()
	request.Uuid = id.String()
}

// Next creates a new request based on the previous one.
func (request *Request) Next(command string, parameters key_value.KeyValue) {
	request.Command = command
	request.Parameters = parameters
}

// Fail creates a new Reply as a failure
// It accepts the error message that explains the reason of the failure.
func (request *Request) Fail(message string) ReplyInterface {
	reply := &Reply{
		Status:     FAIL,
		Message:    message,
		Parameters: key_value.New(),
		Uuid:       request.Uuid,
		conId:      request.conId,
		Trace:      request.Trace,
	}

	return reply
}

func (request *Request) Ok(parameters key_value.KeyValue) ReplyInterface {
	reply := &Reply{
		Status:     OK,
		Message:    "",
		Parameters: parameters,
		Trace:      request.Trace,
		Uuid:       request.Uuid,
		conId:      request.conId,
	}

	return reply
}

func (request *Request) SetMeta(meta map[string]string) {
	pubKey, ok := meta["pub_key"]
	if ok {
		request.SetPublicKey(pubKey)
	}
}
