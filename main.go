package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/builder"
	"github.com/jhump/protoreflect/dynamic"
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/types/known/anypb"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "online" && os.Args[1] != "offline") {
		fmt.Println("Usage: 'go run main.go online' or 'go run main.go offline'", os.Args[1])
		os.Exit(1)
	}
	if os.Args[1] == "offline" {
		offlineParseAndMarshal()
	} else {
		onlineParseAndMarshal()
	}
}

func offlineParseAndMarshal() {
	// Message descriptor for Any type, to use in the content field of the Proposal.
	anyDesc, err := desc.LoadMessageDescriptorForMessage(&anypb.Any{})
	if err != nil {
		panic(err)
	}

	// Proposal type. Only setting the uint proposal_id field
	// and the Any content field, as that suffices to demonstrate the issue.
	mbProposal := builder.NewMessage("Proposal").AddField(
		builder.NewField(
			"proposal_id",
			builder.FieldTypeUInt64(),
		).SetNumber(1),
	).AddField(
		builder.NewField(
			"content",
			builder.FieldTypeImportedMessage(anyDesc),
		).SetNumber(2),
	)
	// ProposalsResponse wraps the Proposal type.
	respDesc, err := builder.NewMessage("ProposalsResponse").AddField(
		builder.NewField(
			"proposals",
			builder.FieldTypeMessage(mbProposal),
		).SetNumber(1),
	).Build()
	if err != nil {
		panic(err)
	}

	// Binary response from server on a previous online run.
	const inb64 = `CtADCAESwAIKIC9jb3Ntb3MuZ292LnYxYmV0YTEuVGV4dFByb3Bvc2FsEpsCCiNXZSB3YW50IHRvIGNsYWltIHJld2FyZHMgY29tcGxldGVseRLzAVJld2FyZC1jbGFpbWluZyB0ZXh0IHByb3Bvc2FsCgpQbGVhc2UgZGVwb3NpdCBhbmQgdm90ZSBmb3IgdGhpcyBwcm9wb3NhbCBpZiB5b3Ugd2FudCB0byBjb21wbGV0ZSAxMDAlIG9mIHlvdXIgZHJvcCBjaGFsbGVuZ2UgQVNBUC4KClRoYW5rcyBPc21vc2lzIHRlYW0gZm9yIEtZQy1mcmVlIGFuZCBuby1idWxsc2hpdCBkcm9wLgpMZXRzIGJ1aWxkIHRoZSBzdHJvbmcgY29tbXVuaXR5IGFuZCBiZXR0ZXIgd2ViIHRvZ2V0aGVyIRgDIjoKDTQ0ODE5NjA0MzQ2NjYSDTEzNzE3MTc1ODE5MjQaDDYxNjAzNTcwOTk0MiIMMTEwMDYwNjYwNDI3KgwIuIe5hgYQtoCb6AEyDAi48YKHBhC2gJvoAToTCgV1b3NtbxIKMjUxMDAwMDAwMEIMCMrguYYGEJT7/aADSgwIysnJhgYQlPv9oAMSCgoIAAAAAAAAAAI=`
	b, err := base64.StdEncoding.DecodeString(inb64)
	if err != nil {
		panic(err)
	}

	parsed := dynamic.NewMessage(respDesc)
	if err := parsed.Unmarshal(b); err != nil {
		panic(err)
	}

	// Text marshaling seems to work fine.
	fmt.Println("Text marshal:")
	t, err := parsed.MarshalText()
	if err != nil {
		fmt.Println("Failed to MarshalText:", err)
	} else {
		fmt.Println(string(t))
	}

	// JSON marshaling panics due to an internal wire type error.
	fmt.Println()
	fmt.Println("JSON marshal:")
	j, err := parsed.MarshalJSONPB(&jsonpb.Marshaler{
		AnyResolver: textProposalResolver{},
	})
	if err != nil {
		fmt.Println("Failed to MarshalJSONPB:", err)
	} else {
		fmt.Println(string(j))
	}
}

// textProposalResolver just resolves the TextProposal type as a dynamic, empty message.
// This ought to treat all the provided fields as unknown/ignored.
type textProposalResolver struct{}

func (r textProposalResolver) Resolve(typeURL string) (proto.Message, error) {
	if typeURL != "/cosmos.gov.v1beta1.TextProposal" {
		panic("textProposalResolver cannot resolve " + typeURL)
	}

	mbTextProposal, err := builder.NewMessage("TextProposal").Build()
	if err != nil {
		panic(fmt.Errorf("failed to build text proposal: %w", err))
	}
	return mbTextProposal.AsProto(), nil
}

// onlineParseAndMarshal connects to the live gRPC server
// and attempts to resolve and parse the proposals message containing an Any field.
func onlineParseAndMarshal() {
	ctx := context.Background()
	const grpcAddr = "osmosis.strange.love:9090"

	conn, err := grpc.DialContext(ctx, grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(fmt.Errorf("failed to dial gRPC address %q: %w", grpcAddr, err))
	}

	stub := rpb.NewServerReflectionClient(conn)
	c := grpcreflect.NewClient(ctx, stub)
	defer c.Reset()

	svcDesc, err := c.ResolveService("cosmos.gov.v1beta1.Query")
	if err != nil {
		panic(err)
	}

	methodDesc := svcDesc.FindMethodByName("Proposals")
	if methodDesc == nil {
		panic("method not found")
	}

	inMsgDesc := methodDesc.GetInputType()
	inputMsg := dynamic.NewMessage(inMsgDesc)
	if err := inputMsg.UnmarshalJSON([]byte(`{"pagination": {"limit":1}}`)); err != nil {
		panic(err)
	}

	dynClient := grpcdynamic.NewStub(conn)
	output, err := dynClient.InvokeRpc(ctx, methodDesc, inputMsg)
	if err != nil {
		panic(fmt.Errorf("failed to invoke rpc: %w", err))
	}
	dynOutput, err := dynamic.AsDynamicMessage(output)
	if err != nil {
		panic(err)
	}

	fmt.Println("Text marshal:")
	t, err := dynOutput.MarshalText()
	if err != nil {
		fmt.Println("Failed to MarshalText:", err)
	} else {
		fmt.Println(string(t))
	}

	fmt.Println()

	fmt.Println("JSON marshal:")
	j, err := dynOutput.MarshalJSONPB(&jsonpb.Marshaler{
		// For Any fields, resolve through the client.
		AnyResolver: reflectClientAnyResolver{c: c},
	})
	if err != nil {
		fmt.Println("Failed to MarshalJSONPB:", err)
	} else {
		fmt.Println(string(j))
	}
}

// reflectClientAnyResolver uses the reflection client
// to query the server to resolve a new Any type.
type reflectClientAnyResolver struct {
	c *grpcreflect.Client
}

var _ jsonpb.AnyResolver = reflectClientAnyResolver{}

func (r reflectClientAnyResolver) Resolve(typeURL string) (proto.Message, error) {
	// Unclear if it is always safe to trim the leading slash here.
	typeURL = strings.TrimPrefix(typeURL, "/")
	d, err := r.c.ResolveMessage(typeURL)
	if err != nil {
		return nil, err
	}

	return d.AsProto(), nil
}
