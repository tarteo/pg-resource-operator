package v1

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PostgresReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

func (r *PostgresReference) GetPostgressObject(ctx context.Context, c client.Client, ownerNamespace string) (Postgres, error) {
	var postgres Postgres

	var namespace string = ownerNamespace
	if r.Namespace != "" {
		namespace = r.Namespace
	}

	err := c.Get(ctx, types.NamespacedName{
		Name:      r.Name,
		Namespace: namespace,
	}, &postgres)

	return postgres, err
}

func (r *PostgresReference) GetConnectionURI(ctx context.Context, c client.Client, ownerNamespace string) (string, error) {
	// Get Postgres object
	postgres, err := r.GetPostgressObject(ctx, c, ownerNamespace)
	if err != nil {
		return "", err
	}

	// Get secret
	secretRef := postgres.Spec.Secret
	// Determine the namespace to look for the secret in, the priority is: secretRef.Namespace > PostgresReference.Namespace > ownerNamespace
	var namespace string = ownerNamespace
	if secretRef.Namespace != "" {
		namespace = secretRef.Namespace
	} else if r.Namespace != "" {
		namespace = r.Namespace
	}
	var secret corev1.Secret
	err = c.Get(ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: namespace,
	}, &secret)
	if err != nil {
		return "", err
	}

	// Extract directly from URI key in the secret if it exists, otherwise construct the URI from the individual components
	if uri, ok := secret.Data[postgres.Spec.URIKey]; ok {
		return string(uri), nil
	}

	// Get secret data for constructing the URI
	// First check if host and port are specified directly in the Postgres spec, if not, look them up in the secret using the specified keys
	var hostString string = postgres.Spec.Host
	if hostString == "" {
		host, ok := secret.Data[postgres.Spec.HostKey]
		if !ok {
			return "", fmt.Errorf("host key not found in secret: %s", postgres.Spec.HostKey)
		}
		hostString = string(host)
	}
	var portString string = strconv.Itoa(int(postgres.Spec.Port))
	if portString == "" {
		port, ok := secret.Data[postgres.Spec.PortKey]
		if !ok {
			return "", fmt.Errorf("port key not found in secret: %s", postgres.Spec.PortKey)
		}
		portString = string(port)
	}
	var usernameString string = postgres.Spec.Username
	if usernameString == "" {
		username, ok := secret.Data[postgres.Spec.UsernameKey]
		if !ok {
			return "", fmt.Errorf("username key not found in secret: %s", postgres.Spec.UsernameKey)
		}
		usernameString = string(username)
	}
	password, ok := secret.Data[postgres.Spec.PasswordKey]
	if !ok {
		return "", fmt.Errorf("password key not found in secret: %s", postgres.Spec.PasswordKey)
	}
	database := postgres.Spec.DefaultDatabase

	// Construct the URI
	sslMode := postgres.Spec.SSLMode
	uri := "postgres://" + usernameString + ":" + string(password) + "@" + hostString + ":" + portString + "/" + database + "?sslmode=" + sslMode
	return uri, nil
}

func (r *PostgresReference) GetPostgresHandle(ctx context.Context, c client.Client, ownerNamespace string) (*sql.DB, error) {
	// Get the connection URI
	uri, err := r.GetConnectionURI(ctx, c, ownerNamespace)
	if err != nil {
		return nil, err
	}

	// Use the URI to create a DB handle
	db, err := sql.Open("postgres", uri)
	if err != nil {
		return nil, err
	}
	return db, nil
}
