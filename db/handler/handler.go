// Package handler lists the commands for database service.
package handler

import (
	"fmt"

	"github.com/blocklords/sds/app/command"
	"github.com/blocklords/sds/common/data_type/key_value"

	zmq "github.com/pebbe/zmq4"
)

const (
	NEW_CREDENTIALS command.CommandName = "new-credentials" // for pull controller, to receive credentials from vault
	SELECT_ROW      command.CommandName = "select-row"      // Get one row, if it doesn't exist, return error
	SELECT_ALL      command.CommandName = "select"          // Read multiple line
	INSERT          command.CommandName = "insert"          // insert new row
	EXIST           command.CommandName = "exist"           // Returns true or false if select query has some rows
	DELETE          command.CommandName = "delete"          // Delete some rows from database
)

// DatabaseQueryRequest has the sql and it's parameters on part with commands.
type DatabaseQueryRequest struct {
	// Fields to manipulate,
	// for reading, it will have the SELECT clause fields
	//
	// for writing, it will have the INSERT VALUES() clause fields
	Fields    []string      `json:"fields"`
	Tables    []string      `json:"tables"`              // Tables that are used for query
	Where     string        `json:"where,omitempty"`     // WHERE part of the SQL query
	Arguments []interface{} `json:"arguments,omitempty"` // to pass in where clause
}

// SelectRowReply keeps the parameters of READ_ROW command reply by controller
type SelectRowReply struct {
	Outputs key_value.KeyValue `json:"outputs"` // all column parameters returned back to user
}

// SelectAllReply keeps the parameters of READ_ALL command reply by controller
type SelectAllReply struct {
	Rows []key_value.KeyValue `json:"rows"` // list of rows returned back to user
}

// InsertReply keeps the parameters of WRITE command reply by controller
type InsertReply struct{}

// ExistReply keeps the parameters of EXIST command reply by controller
type ExistReply struct {
	Exist bool `json:"exist"` // true or false
}

// DeleteReply keeps the parameters of DELETE command reply by controller
type DeleteReply struct{}

// PullerEndpoint returns the inproc pull controller to
// database.
//
// The pull controller receives the message from database
func PullerEndpoint() string {
	return "inproc://database_renew"
}

func PushSocket() (*zmq.Socket, error) {
	sock, err := zmq.NewSocket(zmq.PUSH)
	if err != nil {
		return nil, fmt.Errorf("zmq error for new push socket: %w", err)
	}

	if err := sock.Connect(PullerEndpoint()); err != nil {
		return nil, fmt.Errorf("socket.Connect: %s: %w", PullerEndpoint(), err)
	}

	return sock, nil
}
