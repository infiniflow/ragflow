//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package opengauss

import (
	"context"
	"database/sql"
	"fmt"
	"slices"

	pg "github.com/pganalyze/pg_query_go/v6"
	parser "github.com/wasilibs/go-pgquery"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func parseSelect(query string) (*pg.ParseResult, *pg.SelectStmt, error) {
	tree, err := parser.Parse(query)
	if err != nil {
		return nil, nil, err
	}
	if len(tree.Stmts) != 1 || tree.Stmts[0].Stmt.GetSelectStmt() == nil {
		return nil, nil, fmt.Errorf("opengauss: expected one SELECT statement")
	}
	return tree, tree.Stmts[0].Stmt.GetSelectStmt(), nil
}

// ScopeChatSQL accepts a single-table report over an authorized tenant's chunk store.
// The query and the dataset predicate are composed as syntax trees and emitted together.
func ScopeChatSQL(query string, datasets map[string][]string) (string, bool) {
	tree, stmt, err := parseSelect(query)
	if err != nil || len(stmt.FromClause) != 1 || stmt.WithClause != nil || stmt.Larg != nil || stmt.Rarg != nil ||
		stmt.IntoClause != nil || len(stmt.LockingClause) != 0 {
		return "", false
	}
	table := stmt.FromClause[0].GetRangeVar()
	if table == nil || table.Schemaname != "" || table.Catalogname != "" || len(table.GetAlias().GetColnames()) != 0 {
		return "", false
	}
	ids := datasets[table.Relname]
	if len(ids) == 0 || !reportExpression(stmt.ProtoReflect()) {
		return "", false
	}
	values := make([]*pg.Node, 0, len(ids))
	for _, id := range ids {
		values = append(values, pg.MakeAConstStrNode(id, -1))
	}
	scope := pg.MakeAExprNode(pg.A_Expr_Kind_AEXPR_IN, []*pg.Node{pg.MakeStrNode("=")},
		pg.MakeColumnRefNode([]*pg.Node{pg.MakeStrNode("kb_id")}, -1), pg.MakeListNode(values), -1)
	if stmt.WhereClause != nil {
		scope = pg.MakeBoolExprNode(pg.BoolExprType_AND_EXPR, []*pg.Node{stmt.WhereClause, scope}, -1)
	}
	stmt.WhereClause = scope
	sqlText, err := parser.Deparse(tree)
	return sqlText, err == nil
}

// Only scalar report expressions and built-in aggregates can operate on the scoped rows.
func reportExpression(msg protoreflect.Message) bool {
	switch node := msg.Interface().(type) {
	case *pg.FuncCall:
		switch len(node.Funcname) {
		case 1:
			node.Funcname = append([]*pg.Node{pg.MakeStrNode("pg_catalog")}, node.Funcname...)
		case 2:
			if node.Funcname[0].GetString_().GetSval() != "pg_catalog" {
				return false
			}
		default:
			return false
		}
		if !slices.Contains([]string{"count", "sum", "avg", "min", "max", "round", "lower", "upper", "length"}, node.Funcname[1].GetString_().GetSval()) {
			return false
		}
	case *pg.SortBy:
		if len(node.UseOp) != 0 {
			return false
		}
	case *pg.A_Expr:
		if len(node.Name) != 1 || !slices.Contains([]string{"=", "<>", "<", ">", "<=", ">=", "+", "-", "*", "/", "%", "~~", "!~~", "~~*", "!~~*", "BETWEEN", "NOT BETWEEN"}, node.Name[0].GetString_().GetSval()) {
			return false
		}
	case *pg.SelectStmt, *pg.Node, *pg.RangeVar, *pg.Alias, *pg.ResTarget, *pg.ColumnRef, *pg.A_Star, *pg.A_Const,
		*pg.Integer, *pg.Float, *pg.Boolean, *pg.String, *pg.List, *pg.BoolExpr, *pg.NullTest, *pg.BooleanTest,
		*pg.CaseExpr, *pg.CaseWhen, *pg.CoalesceExpr:
	default:
		return false
	}
	ok := true
	msg.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if !reportExpression(list.Get(i).Message()) {
					ok = false
					break
				}
			}
		} else {
			ok = reportExpression(value.Message())
		}
		return ok
	})
	return ok
}

// RunSQL executes a single read-only report with a default outer row limit.
func (e *Engine) RunSQL(ctx context.Context, sqlText string, format string) ([]map[string]interface{}, error) {
	tree, stmt, err := parseSelect(sqlText)
	if err != nil {
		return nil, err
	}
	if stmt.LimitCount == nil || stmt.LimitCount.GetAConst().GetIsnull() {
		stmt.LimitCount = pg.MakeAConstIntNode(1024, -1)
		stmt.LimitOption = pg.LimitOption_LIMIT_OPTION_COUNT
	}
	query, err := parser.Deparse(tree)
	if err != nil {
		return nil, err
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return queryRowsTx(ctx, tx, query)
}
