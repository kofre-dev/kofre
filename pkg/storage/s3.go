package storage

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// S3Config contem as configuracoes para conexao com AWS S3 ou Cloudflare R2
type S3Config struct {
	Endpoint        string // opcional: ex: "https://<accountid>.r2.cloudflarestorage.com" ou vazio para AWS padrao
	Region          string // ex: "us-east-1" ou "auto"
	Bucket          string // nome do bucket
	Key             string // caminho do arquivo no bucket (ex: "vault.enc")
	AccessKeyID     string // AWS / R2 Access Key ID
	SecretAccessKey string // AWS / R2 Secret Access Key
	PresignedGetURL string // se usar gateway com URL pre-assinada
	PresignedPutURL string // se usar gateway com URL pre-assinada
}

// S3Storage implementa o StorageProvider conectando a AWS S3 ou compatíveis (Cloudflare R2, MinIO)
type S3Storage struct {
	config     S3Config
	httpClient *http.Client
}

// NewS3Storage cria o provider para S3
func NewS3Storage(cfg S3Config) *S3Storage {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if cfg.Key == "" {
		cfg.Key = "vault.enc"
	}
	return &S3Storage{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *S3Storage) Load(ctx context.Context) ([]byte, error) {
	if s.config.PresignedGetURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.config.PresignedGetURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("falha ao requisitar arquivo do S3 via presigned URL: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("status inesperado ao baixar do S3: %s", resp.Status)
		}

		return io.ReadAll(resp.Body)
	}

	// Requisicao direta com SigV4
	endpoint := s.resolveEndpoint()
	targetURL := fmt.Sprintf("%s/%s", endpoint, s.config.Key)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	if s.config.AccessKeyID != "" && s.config.SecretAccessKey != "" {
		s.signV4(req, []byte{})
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar ao S3: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("erro S3 HTTP %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

func (s *S3Storage) Save(ctx context.Context, data []byte) error {
	if s.config.PresignedPutURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.config.PresignedPutURL, bytes.NewReader(data))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("falha ao enviar arquivo para o S3 via presigned URL: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("status inesperado ao salvar no S3: %s", resp.Status)
		}
		return nil
	}

	endpoint := s.resolveEndpoint()
	targetURL := fmt.Sprintf("%s/%s", endpoint, s.config.Key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, targetURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	if s.config.AccessKeyID != "" && s.config.SecretAccessKey != "" {
		s.signV4(req, data)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao gravar no S3: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("erro S3 PUT HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Delete remove o objeto do bucket S3
func (s *S3Storage) Delete(ctx context.Context) error {
	s3URL := fmt.Sprintf("%s/%s", s.resolveEndpoint(), s.config.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s3URL, nil)
	if err != nil {
		return err
	}
	s.signV4(req, []byte{})
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("falha ao deletar do S3: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

func (s *S3Storage) Exists(ctx context.Context) (bool, error) {
	_, err := s.Load(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (s *S3Storage) Location() string {
	return fmt.Sprintf("s3://%s/%s", s.config.Bucket, s.config.Key)
}

func (s *S3Storage) resolveEndpoint() string {
	if s.config.Endpoint != "" {
		ep := strings.TrimRight(s.config.Endpoint, "/")
		return fmt.Sprintf("%s/%s", ep, s.config.Bucket)
	}
	// Se o bucket contiver pontos (ex: kofre.dev), o certificado wildcard da AWS (*.s3.region.amazonaws.com)
	// não cobre subdomínios de múltiplos níveis. Usamos path-style addressing oficial da AWS.
	if strings.Contains(s.config.Bucket, ".") {
		return fmt.Sprintf("https://s3.%s.amazonaws.com/%s", s.config.Region, s.config.Bucket)
	}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com", s.config.Bucket, s.config.Region)
}

// signV4 aplica autenticacao AWS SigV4 basica
func (s *S3Storage) signV4(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	payloadHash := sha256.Sum256(payload)
	payloadHashHex := hex.EncodeToString(payloadHash[:])

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHashHex)
	req.Header.Set("Host", req.URL.Host)

	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		req.URL.Host, payloadHashHex, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method, canonicalURI, req.URL.RawQuery, canonicalHeaders, signedHeaders, payloadHashHex)

	canonicalReqHash := sha256.Sum256([]byte(canonicalRequest))
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.config.Region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate, credentialScope, hex.EncodeToString(canonicalReqHash[:]))

	kDate := hmacSHA256([]byte("AWS4"+s.config.SecretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.config.Region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		s.config.AccessKeyID, credentialScope, signedHeaders, signature)

	req.Header.Set("Authorization", authHeader)
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
