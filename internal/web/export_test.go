package web

import "github.com/z354prance/undermynd/internal/config"

func ValidDonationsForTest(d *config.Donations) error { return validDonations(d) }
