package domain

type ExtractedData struct {
	DCID    int
	AuthKey []byte
	UserID  int64
}

type EncryptedResult struct {
	DCID             int
	AuthKeyEncrypted string
	UserID           int64
}

type DatacenterKey struct {
	DCID    int
	AuthKey []byte
	KeyID   []byte
}
