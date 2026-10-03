package model

import (
	"time"

	"gorm.io/gorm"
)

type WalletTransaction struct {
	Id             string    `gorm:"column:id;primary_key" json:"id"`
	UserId         int       `gorm:"column:user_id;not null;index:idx_wallet_tx_user_created" json:"user_id"`
	Type           string    `gorm:"column:type;type:varchar(20);not null" json:"type"`
	Amount         int64     `gorm:"column:amount;not null" json:"amount"`
	IdempotencyKey string    `gorm:"column:idempotency_key;type:varchar(128);uniqueIndex:idx_wallet_tx_idempotency;not null" json:"idempotency_key"`
	DeductionId    string    `gorm:"column:deduction_id;type:varchar(64)" json:"deduction_id,omitempty"`
	Metadata       string    `gorm:"column:metadata;type:text" json:"metadata,omitempty"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime;index:idx_wallet_tx_user_created" json:"created_at"`
}

func (WalletTransaction) TableName() string {
	return "wallet_transactions"
}

func CreateWalletTransaction(tx *WalletTransaction) error {
	return DB.Create(tx).Error
}

func GetTransactionByIdempotencyKey(key string) (*WalletTransaction, error) {
	var tx WalletTransaction
	err := DB.Where("idempotency_key = ?", key).First(&tx).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &tx, err
}
