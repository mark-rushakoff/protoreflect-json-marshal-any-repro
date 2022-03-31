# protoreflect-json-marshal-any-repro

This repository contains a reproducer for a dynamic message from protoreflect failing to MarshalJSON.

`go run main.go offline` runs the offline reproducer, which uses a locally defined dynamic message
to parse a binary protobuf message.
It successfully text-marshals the message, but it fails to JSON-marshal it with an error
`proto: cannot parse invalid wire-format data`.

`go run main.go online` runs the online reproducer which connects to a (hardcoded) live gRPC server,
uses a reflection client to learn the message types, send a unary request,
and parse the response with a dynamic message.
The parse is successful, and it can text-marshal the message,
but it fails to JSON-marshal the response message the same way as the offline reproducer.
