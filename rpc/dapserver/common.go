package dapserver

import dap "github.com/google/go-dap"

func newEvent(event string) dap.Event {
	return dap.Event{
		ProtocolMessage: dap.ProtocolMessage{
			Seq:  0,
			Type: "event",
		},
		Event: event,
	}
}

func newResponse(msg dap.RequestMessage) dap.Response {
	req := msg.GetRequest()
	return dap.Response{
		ProtocolMessage: dap.ProtocolMessage{
			Seq:  0,
			Type: "response",
		},
		Command:    req.Command,
		RequestSeq: req.Seq,
		Success:    true,
	}
}

func newErrorResponse(msg dap.RequestMessage, err error) *dap.ErrorResponse {
	resp := &dap.ErrorResponse{
		Response: newResponse(msg),
	}
	resp.Success = false
	resp.Message = err.Error()
	return resp
}
