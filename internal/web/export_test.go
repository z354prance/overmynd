package web

import "github.com/z354prance/overmynd/internal/config"

func ValidDonationsForTest(d *config.Donations) error { return validDonations(d) }
