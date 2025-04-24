package tests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoBufferedChannels(t *testing.T) {
	filesToCheck := []string{
		"../filecrawler/crawler.go",
		"../workerpool/pool.go",
	}

	for _, relPath := range filesToCheck {
		absPath, err := filepath.Abs(relPath)
		require.NoError(t, err)

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, absPath, nil, parser.AllErrors)
		require.NoError(t, err)

		ast.Inspect(node, func(n ast.Node) bool {
			if makeExpr, ok := n.(*ast.CallExpr); ok {
				if ident, ok := makeExpr.Fun.(*ast.Ident); ok && ident.Name == "make" {
					if _, ok := makeExpr.Args[0].(*ast.ChanType); ok {
						require.Equal(t, 1, len(makeExpr.Args),
							"File %s contains a buffered channel at position %v",
							relPath, fset.Position(makeExpr.Pos()))
					}
				}
			}

			return true
		})
	}
}
