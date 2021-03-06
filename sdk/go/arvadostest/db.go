// Copyright (C) The Arvados Authors. All rights reserved.
//
// SPDX-License-Identifier: Apache-2.0

package arvadostest

import (
	"context"

	"git.arvados.org/arvados.git/lib/ctrlctx"
	"git.arvados.org/arvados.git/sdk/go/arvados"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"gopkg.in/check.v1"
)

// DB returns a DB connection for the given cluster config.
func DB(c *check.C, cluster *arvados.Cluster) *sqlx.DB {
	db, err := sqlx.Open("postgres", cluster.PostgreSQL.Connection.String())
	c.Assert(err, check.IsNil)
	return db
}

// TransactionContext returns a context suitable for running a test
// case in a new transaction, and a rollback func which the caller
// should call after the test.
func TransactionContext(c *check.C, db *sqlx.DB) (ctx context.Context, rollback func()) {
	tx, err := db.Beginx()
	c.Assert(err, check.IsNil)
	return ctrlctx.NewWithTransaction(context.Background(), tx), func() {
		c.Check(tx.Rollback(), check.IsNil)
	}
}
