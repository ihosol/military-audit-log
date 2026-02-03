package storage

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinioStorage struct {
	client     *minio.Client
	bucketName string
}

func NewMinioStorage(endpoint, accessKey, secretKey, bucket string) *MinioStorage {
	// 1. Ініціалізація клієнта
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false, // Для локального тестування без HTTPS
	})
	if err != nil {
		log.Fatalln(err)
	}

	// 2. Створення бакета (папки), якщо немає
	ctx := context.Background()
	exists, err := minioClient.BucketExists(ctx, bucket)
	if err == nil && !exists {
		minioClient.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
		fmt.Printf("Created bucket: %s\n", bucket)
	}

	return &MinioStorage{
		client:     minioClient,
		bucketName: bucket,
	}
}

func (s *MinioStorage) Upload(filename string, data io.Reader, size int64) (string, error) {
	ctx := context.Background()

	// Завантаження потоку даних
	info, err := s.client.PutObject(ctx, s.bucketName, filename, data, size, minio.PutObjectOptions{
		ContentType: "application/pdf", // Можна змінювати динамічно
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s/%s", s.bucketName, info.Key), nil
}
