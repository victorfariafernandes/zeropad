package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/common/auth"
	"github.com/oracle/oci-go-sdk/v65/objectstorage"
)

// defaultPrefix is prepended to every object name written to OCI. It maps to
// the "default-2-day-ttl" object lifecycle rule on the pads bucket
// (infra/terraform/modules/storage/main.tf), which deletes objects under
// this prefix 2 days after their last-modified time. A future TTL tier
// would use a different prefix backed by its own lifecycle rule.
const defaultPrefix = "default/"

// DefaultPadTTL is how long a pad written under defaultPrefix survives
// before OCI's lifecycle rule deletes it ("default-2-day-ttl", time_amount=2
// / time_unit=DAYS). It is used to compute the expires_at surfaced to
// clients; it does not itself trigger deletion — OCI's own lifecycle policy
// does that, based on object Last-Modified time, which is why Pad.UpdatedAt
// must be refreshed on every write. A future TTL tier would pair a new
// prefix + lifecycle rule with its own constant here.
const DefaultPadTTL = 48 * time.Hour

// OCIPadStore implements PadStore backed by OCI Object Storage.
// Each pad is stored as a JSON-encoded object named by defaultPrefix+slug.
type OCIPadStore struct {
	client    objectstorage.ObjectStorageClient
	namespace string
	bucket    string
}

// NewOCIPadStore creates an OCIPadStore. Tries Instance Principal auth first
// (production); falls back to API key env vars for local dev.
func NewOCIPadStore(namespace, bucket string) (*OCIPadStore, error) {
	provider, err := auth.InstancePrincipalConfigurationProvider()
	if err != nil {
		provider, err = apiKeyProviderFromEnv()
		if err != nil {
			return nil, fmt.Errorf("store: no valid OCI auth provider: %w", err)
		}
	}
	client, err := objectstorage.NewObjectStorageClientWithConfigurationProvider(provider)
	if err != nil {
		return nil, fmt.Errorf("store: create OCI client: %w", err)
	}
	return &OCIPadStore{client: client, namespace: namespace, bucket: bucket}, nil
}

func (s *OCIPadStore) Get(slug string) (Pad, bool) {
	req := objectstorage.GetObjectRequest{
		NamespaceName: common.String(s.namespace),
		BucketName:    common.String(s.bucket),
		ObjectName:    common.String(defaultPrefix + slug),
	}
	resp, err := s.client.GetObject(context.Background(), req)
	if err != nil {
		if svcErr, ok := common.IsServiceError(err); ok && svcErr.GetHTTPStatusCode() == http.StatusNotFound {
			return Pad{}, false
		}
		log.Printf("store: GetObject %q: %v", slug, err)
		return Pad{}, false
	}
	defer resp.Content.Close()

	var pad Pad
	if err := json.NewDecoder(resp.Content).Decode(&pad); err != nil {
		log.Printf("store: decode pad %q: %v", slug, err)
		return Pad{}, false
	}
	return pad, true
}

func (s *OCIPadStore) Set(slug string, pad Pad) {
	body, err := json.Marshal(pad)
	if err != nil {
		log.Printf("store: marshal pad %q: %v", slug, err)
		return
	}
	req := objectstorage.PutObjectRequest{
		NamespaceName: common.String(s.namespace),
		BucketName:    common.String(s.bucket),
		ObjectName:    common.String(defaultPrefix + slug),
		ContentType:   common.String("application/json"),
		ContentLength: common.Int64(int64(len(body))),
		PutObjectBody: io.NopCloser(bytes.NewReader(body)),
	}
	if _, err := s.client.PutObject(context.Background(), req); err != nil {
		log.Printf("store: PutObject %q: %v", slug, err)
	}
}

func (s *OCIPadStore) Delete(slug string) {
	req := objectstorage.DeleteObjectRequest{
		NamespaceName: common.String(s.namespace),
		BucketName:    common.String(s.bucket),
		ObjectName:    common.String(defaultPrefix + slug),
	}
	if _, err := s.client.DeleteObject(context.Background(), req); err != nil {
		if svcErr, ok := common.IsServiceError(err); ok && svcErr.GetHTTPStatusCode() == http.StatusNotFound {
			return
		}
		log.Printf("store: DeleteObject %q: %v", slug, err)
	}
}

// apiKeyProviderFromEnv builds an API key config from environment variables.
// Env vars: OCI_TENANCY_OCID, OCI_USER_OCID, OCI_FINGERPRINT,
// OCI_PRIVATE_KEY (PEM content), OCI_REGION, OCI_PRIVATE_KEY_PASSPHRASE (optional).
func apiKeyProviderFromEnv() (common.ConfigurationProvider, error) {
	tenancy := os.Getenv("OCI_TENANCY_OCID")
	user := os.Getenv("OCI_USER_OCID")
	fingerprint := os.Getenv("OCI_FINGERPRINT")
	privKey := os.Getenv("OCI_PRIVATE_KEY")
	region := os.Getenv("OCI_REGION")
	if tenancy == "" || user == "" || fingerprint == "" || privKey == "" || region == "" {
		return nil, errors.New("OCI API key env vars not set (OCI_TENANCY_OCID, OCI_USER_OCID, OCI_FINGERPRINT, OCI_PRIVATE_KEY, OCI_REGION)")
	}
	passphrase := os.Getenv("OCI_PRIVATE_KEY_PASSPHRASE")
	return common.NewRawConfigurationProvider(tenancy, user, region, fingerprint, privKey, &passphrase), nil
}
