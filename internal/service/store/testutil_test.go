package store

import "time"

var testPacksReadyAt = time.Unix(1, 0).UTC()

func packsReady() *time.Time { return &testPacksReadyAt }
