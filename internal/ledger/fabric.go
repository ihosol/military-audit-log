package ledger

import (
	"crypto/x509"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/hyperledger/fabric-gateway/pkg/client"
	"github.com/hyperledger/fabric-gateway/pkg/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type FabricLedger struct {
	clientConnection *grpc.ClientConn
	gateway          *client.Gateway
	contract         *client.Contract
}

// NewFabricLedger підключається до локальної мережі Fabric
func NewFabricLedger() (*FabricLedger, error) {
	// Шляхи до криптографії (зміни, якщо папка fabric-network в іншому місці)
	mspID := "Org1MSP"
	cryptoPath := "../fabric-network/fabric-samples/test-network/organizations/peerOrganizations/org1.example.com"

	if _, err := os.Stat(cryptoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("crypto path does not exist: %s", cryptoPath)
	}

	certPath := path.Join(cryptoPath, "users/User1@org1.example.com/msp/signcerts/cert.pem")
	keyDir := path.Join(cryptoPath, "users/User1@org1.example.com/msp/keystore")
	tlsCertPath := path.Join(cryptoPath, "peers/peer0.org1.example.com/tls/ca.crt")

	// 1. Завантаження сертифікату
	cert, err := loadCertificate(certPath)
	if err != nil {
		return nil, err
	}

	// 2. Завантаження ключа
	key, err := loadPrivateKey(keyDir)
	if err != nil {
		return nil, err
	}

	// --- ВИПРАВЛЕННЯ ТУТ ---
	// Створення Identity (тепер повертає помилку і приймає лише 2 аргументи)
	id, err := identity.NewX509Identity(mspID, cert)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity: %w", err)
	}

	// Створення Signer (тепер повертає помилку)
	sign, err := identity.NewPrivateKeySign(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}
	// -----------------------

	// 3. Підключення gRPC
	transportCreds, err := credentials.NewClientTLSFromFile(tlsCertPath, "peer0.org1.example.com")
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient("localhost:7051", grpc.WithTransportCredentials(transportCreds))
	if err != nil {
		return nil, err
	}

	// 4. Підключення до Gateway
	gateway, err := client.Connect(
		id,
		client.WithSign(sign),
		client.WithClientConnection(conn),
		client.WithEvaluateTimeout(5*time.Second),
		client.WithEndorseTimeout(15*time.Second),
		client.WithSubmitTimeout(5*time.Second),
		client.WithCommitStatusTimeout(1*time.Minute),
	)
	if err != nil {
		return nil, err
	}

	network := gateway.GetNetwork("mychannel")
	contract := network.GetContract("basic")

	return &FabricLedger{
		clientConnection: conn,
		gateway:          gateway,
		contract:         contract,
	}, nil
}

// Write записує хеш у блокчейн
func (f *FabricLedger) Write(hash string, metadata string) (string, error) {
	fmt.Println("--> Sending transaction to Hyperledger Fabric...")

	// ВИПРАВЛЕННЯ:
	// Стандартний контракт "basic" має таку структуру:
	// CreateAsset(ID string, Color string, Size int, Owner string, AppraisedValue int)
	
	// Ми адаптуємо ці поля під наш журнал аудиту:
	// 1. ID             -> hash (Унікальний хеш файлу)
	// 2. Color          -> metadata (Тут ми збережемо "File: order.txt") - ЦЕ РЯДОК, ТУТ БЕЗПЕЧНО
	// 3. Size           -> "1" (Просто розмір)
	// 4. Owner          -> "System" (Хто записал)
	// 5. AppraisedValue -> "0" (Має бути ЧИСЛОМ, тому передаємо "0", а не текст)

	_, err := f.contract.SubmitTransaction("CreateAsset", hash, metadata, "1", "System", "0")
	
	if err != nil {
		return "", fmt.Errorf("failed to submit transaction: %w", err)
	}

	return "tx_committed_verified", nil
}

func (f *FabricLedger) Close() {
	f.gateway.Close()
	f.clientConnection.Close()
}

func loadCertificate(filename string) (*x509.Certificate, error) {
	certificatePEM, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %w", err)
	}
	return identity.CertificateFromPEM(certificatePEM)
}

func loadPrivateKey(dir string) (interface{}, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read key directory: %w", err)
	}
	privateKeyPEM, err := os.ReadFile(path.Join(dir, files[0].Name()))
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}
	return identity.PrivateKeyFromPEM(privateKeyPEM)
}