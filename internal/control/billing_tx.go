package control

// Billing transaction type identifiers (stored in billing_transactions.tx_type).
const (
	TxBuyerDepositCredit    = "deposit_credit"
	TxBuyerTokenPurchase    = "token_purchase"
	TxBuyerInferenceDebit   = "inference_debit"
	TxBuyerAdjustment       = "adjustment"
	TxBuyerInferenceTrack   = "inference_tracking"

	TxSellerTokenSaleCredit = "token_sale_credit"
	TxSellerWithdrawalDebit = "withdrawal_debit"
	TxSellerInferenceCredit = "inference_credit"
	TxSellerAdjustment      = "adjustment"
	TxSellerInferenceTrack  = "inference_tracking"
)
