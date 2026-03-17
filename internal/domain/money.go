package domain

// Money represents a monetary amount in the smallest currency unit (e.g. VND dong).
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

func VND(amount int64) Money {
	return Money{Amount: amount, Currency: "VND"}
}

func (m Money) Add(other Money) Money {
	return Money{Amount: m.Amount + other.Amount, Currency: m.Currency}
}

func (m Money) Scale(numerator, denominator int64) Money {
	if denominator == 0 {
		return m
	}
	return Money{
		Amount:   m.Amount * numerator / denominator,
		Currency: m.Currency,
	}
}
