package xjson

import "github.com/mutallipp/llm-proxy/internal/objects"

var (
	EmptyJSON            = []byte("{}")
	NullJSON             = []byte("null")
	EmptyArrayJSON       = []byte("[]")
	EmptyJSONRawMessage  = objects.JSONRawMessage(EmptyJSON)
	EmptyArrayRawMessage = objects.JSONRawMessage(EmptyArrayJSON)
	NullJSONRawMessage   = objects.JSONRawMessage(NullJSON)
)
