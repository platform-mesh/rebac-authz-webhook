package client

import (
	"crypto/tls"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"golang.org/x/oauth2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/credentials/oauth"
)

func MustCreateClientWithSource(openfgaAddr string, source oauth2.TokenSource) openfgav1.OpenFGAServiceClient { // coverage-ignore
	conn, err := grpc.NewClient(openfgaAddr,
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{})),
		grpc.WithPerRPCCredentials(oauth.TokenSource{TokenSource: source}),
	)
	if err != nil {
		panic(err)
	}

	return openfgav1.NewOpenFGAServiceClient(conn)
}

func MustCreateInClusterClient(openfgaAddr string) openfgav1.OpenFGAServiceClient { // coverage-ignore
	conn, err := grpc.NewClient(openfgaAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}

	return openfgav1.NewOpenFGAServiceClient(conn)
}
