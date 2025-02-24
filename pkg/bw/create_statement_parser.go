// Copyright 2025 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strings"
)

const (
	createStatementFileName = "crdb_internal.create_statements.txt"
)

// generateDDLs reads a TSV file from zipContentLocation, filters by dbName, and prints the create_statement.
func generateDDLs(
	ctx context.Context, zipContentLocation, dbName string, outputFileLocation string,
) error {
	// Open the TSV file directly from the given location.
	filePath := fmt.Sprintf("%s/%s", zipContentLocation, createStatementFileName)
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()
	// Create the output file for writing.
	outFile, err := os.Create(fmt.Sprintf("%s/%s.ddl", outputFileLocation, dbName))
	if err != nil {
		return err
	}
	defer func() {
		_ = outFile.Close()
	}()
	// Create a CSV reader configured for TSV (tab-separated values).
	reader := csv.NewReader(file)
	reader.Comma = '\t' // Set delimiter as tab
	reader.LazyQuotes = true

	// Read the header row.
	header, err := reader.Read()
	if err != nil {
		log.Fatalf("Error reading header: %v", err)
	}

	// Map column indexes
	columnIndex := map[string]int{}
	for i, col := range header {
		columnIndex[col] = i
	}

	// Ensure required columns exist
	requiredColumns := []string{
		"database_name", "create_statement", "schema_name", "descriptor_type", "descriptor_name",
	}
	for _, col := range requiredColumns {
		if _, exists := columnIndex[col]; !exists {
			log.Fatalf("Missing expected column %q in TSV file", col)
		}
	}

	// the database ie created is it does not exist
	_, err = outFile.WriteString(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %[1]s;\n\n", dbName))
	if err != nil {
		return err
	}

	// Process the records and write to the output file.
	count := 0
	// statements maintains the list of all create statements executed in sequence
	statements := make([]string, 0)
	// createTableRecord maintains the map of the table name to the index at which the statement is added in
	// statements list. This is done to ensure that we take the entry of the last statement that was used for
	// CREATE. All the previous ones are neglected as the same table could have been dropped and created again.
	createTableRecord := make(map[string]int)
	for {
		// Check if the context has been canceled.
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled")
		default:
		}

		// Read the next record.
		record, err := reader.Read()
		if err != nil {
			break // EOF or error
		}

		// Match the "database_name" column with the provided dbName.
		if record[columnIndex["database_name"]] == dbName && record[columnIndex["descriptor_type"]] == "table" {
			schema := record[columnIndex["schema_name"]]
			// add the database name to the statement
			statement := strings.ReplaceAll(record[columnIndex["create_statement"]],
				fmt.Sprintf(" %s.", schema),
				fmt.Sprintf(" %s.%s.", dbName, schema))
			tableName := record[columnIndex["descriptor_name"]]
			fullTableName := fmt.Sprintf("%s.%s.%s", dbName, schema, tableName)
			// There can be 2 scenarios at this point:
			// 1. The statement to create the table was present, but a new entry is seen. So, we should be removing
			//   the old entry
			// 2. this is a fresh new create.
			if index, ok := createTableRecord[fullTableName]; ok {
				statements = append(statements[:index], statements[index+1:]...)
			} else {
				count++
			}
			statements = append(statements, statement)
			createTableRecord[fullTableName] = len(statements) - 1
		}
	}
	// write all the statements to the file
	for _, s := range statements {
		_, err := outFile.WriteString(s + ";\n\n")
		if err != nil {
			return err
		}

	}

	fmt.Printf("Successfully wrote %d create statements to %s/%s.ddl\n", count, outputFileLocation, dbName)
	return nil
}

func main() {
	ctx := context.Background()
	zipContentLocation := "pkg/bw"
	dbName := "cct_tpcc"
	_ = generateDDLs(ctx, zipContentLocation, dbName, zipContentLocation)
}
