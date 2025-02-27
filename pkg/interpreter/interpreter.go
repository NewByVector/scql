// Copyright 2023 Ant Group Co., Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package interpreter

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/secretflow/scql/pkg/infoschema"
	"github.com/secretflow/scql/pkg/interpreter/optimizer"
	"github.com/secretflow/scql/pkg/interpreter/translator"
	"github.com/secretflow/scql/pkg/parser"
	"github.com/secretflow/scql/pkg/planner/core"
	proto "github.com/secretflow/scql/pkg/proto-gen/scql"
	"github.com/secretflow/scql/pkg/proto-gen/spu"
	"github.com/secretflow/scql/pkg/sessionctx"
	"github.com/secretflow/scql/pkg/sessionctx/stmtctx"
)

type Interpreter struct{}

func NewInterpreter() *Interpreter {
	return &Interpreter{}
}

func (intr *Interpreter) Compile(ctx context.Context, req *proto.CompileQueryRequest) (*proto.CompiledPlan, error) {
	p := parser.New()
	stmts, _, err := p.Parse(req.GetQuery(), "", "")
	if err != nil {
		return nil, err
	}
	if len(stmts) != 1 {
		return nil, fmt.Errorf("only support one query one time, but got %d queries", len(stmts))
	}

	is, err := buildInfoSchemaFromCatalogProto(req.GetCatalog())
	if err != nil {
		return nil, err
	}

	sctx := sessionctx.NewContext()
	sctx.GetSessionVars().StmtCtx = &stmtctx.StatementContext{}
	sctx.GetSessionVars().PlanID = 0
	sctx.GetSessionVars().PlanColumnID = 0
	sctx.GetSessionVars().CurrentDB = req.GetDbName()

	lp, _, err := core.BuildLogicalPlanWithOptimization(ctx, sctx, stmts[0], is)
	if err != nil {
		return nil, err
	}
	selectIntoIssuer := false
	if lp.IntoOpt() != nil {
		if lp.IntoOpt().PartyCode != req.GetIssuer().GetCode() {
			return nil, fmt.Errorf("expect select into issuer party code %s but got %s", req.GetIssuer().GetCode(), lp.IntoOpt().PartyCode)
		}
		selectIntoIssuer = true
	}
	enginesInfo, err := buildEngineInfo(lp, req.GetCatalog(), req.GetDbName(), req.GetIssuer().GetCode(), req.GetIssuerAsParticipant() || selectIntoIssuer)
	if err != nil {
		return nil, err
	}

	t, err := translator.NewTranslator(enginesInfo, req.GetSecurityConf(), req.GetIssuer().GetCode(), &translator.SecurityCompromiseConf{
		RevealGroupMark: req.GetCompileOpts().GetSecurityCompromise().GetRevealGroupMark(),
	})
	if err != nil {
		return nil, err
	}

	ep, err := t.Translate(lp)
	if err != nil {
		return nil, err
	}

	graphChecker := translator.NewGraphChecker()
	if err := graphChecker.Check(ep); err != nil {
		return nil, err
	}

	partitioner := optimizer.NewGraphPartitioner(ep)
	if err := partitioner.NaivePartition(); err != nil {
		return nil, err
	}

	mapper := optimizer.NewGraphMapper(ep, partitioner.SubDAGs)
	mapper.Map()

	plan := buildCompiledPlan(req.GetCompileOpts().GetSpuConf(), ep, mapper.Codes)
	if req.GetCompileOpts().GetDumpExeGraph() {
		plan.Explain = &proto.ExplainInfo{
			ExeGraphDot: ep.DumpGraphviz(),
		}
	}
	plan.Warning = &proto.Warning{
		MayAffectedByGroupThreshold: t.AffectedByGroupThreshold,
	}

	return plan, nil
}

func buildCompiledPlan(spuConf *spu.RuntimeConfig, eGraph *translator.Graph, execPlans map[string]*optimizer.ExecutionPlan) *proto.CompiledPlan {
	plan := &proto.CompiledPlan{
		Schema:         &proto.TableSchema{},
		SpuRuntimeConf: spuConf,
		SubGraphs:      make(map[string]*proto.SubGraph),
	}

	{
		// Fill Schema
		for _, out := range eGraph.OutputNames {
			plan.Schema.Columns = append(plan.Schema.Columns, &proto.ColumnDesc{
				Name: out,
				// TODO: populate Field Type
				// Type: <column data type>
			})
		}
		// Fill Parties
		for _, party := range eGraph.GetParties() {
			plan.Parties = append(plan.Parties, &proto.PartyId{
				Code: party,
			})
		}

		// Fill Subgraphs
		for party, subGraph := range execPlans {
			graphProto := &proto.SubGraph{
				Nodes: make(map[string]*proto.ExecNode),
				Policy: &proto.SchedulingPolicy{
					WorkerNum: int32(subGraph.Policy.WorkerNumber),
					Subdags:   make([]*proto.SubDAG, 0),
				},
				// TODO: populate GraphChecksum
				// GraphChecksum: <checksum>,
			}
			// Fill Nodes
			for k, v := range subGraph.Nodes {
				graphProto.Nodes[strconv.Itoa(k)] = v.ToProto()
			}
			// Fill Policy subdags
			for _, job := range subGraph.Policy.Jobs {
				subdag := &proto.SubDAG{
					Jobs:                     make([]*proto.SubDAG_Job, 0),
					NeedCallBarrierAfterJobs: job.NeedCallBarrierAfterJobs,
				}
				for k, v := range job.Jobs {
					var ids []string
					for _, id := range v {
						ids = append(ids, strconv.Itoa(id))
					}

					j := &proto.SubDAG_Job{
						WorkerId: int32(k),
						NodeIds:  ids,
					}
					subdag.Jobs = append(subdag.Jobs, j)
				}
				graphProto.Policy.Subdags = append(graphProto.Policy.Subdags, subdag)
			}
			plan.SubGraphs[party] = graphProto
		}
	}
	return plan
}

func buildInfoSchemaFromCatalogProto(catalog *proto.Catalog) (infoschema.InfoSchema, error) {
	tables := make([]*infoschema.TableSchema, 0, len(catalog.GetTables()))
	for _, tblEntry := range catalog.GetTables() {
		dbTable, err := core.NewDbTableFromString(tblEntry.GetTableName())
		if err != nil {
			return nil, err
		}
		newTable := infoschema.TableSchema{
			DbName:    dbTable.GetDbName(),
			TableName: dbTable.GetTableName(),
		}
		for _, col := range tblEntry.GetColumns() {
			newTable.Columns = append(newTable.Columns, infoschema.ColumnDesc{
				Name: col.GetName(),
				Type: col.GetType(),
			})
		}
		tables = append(tables, &newTable)
	}
	return infoschema.FromTableSchema(tables)
}

func collectDataSourceNode(lp core.LogicalPlan) []*core.DataSource {
	var result []*core.DataSource
	for _, child := range lp.Children() {
		dsList := collectDataSourceNode(child)
		result = append(result, dsList...)
	}

	if len(lp.Children()) > 0 {
		return result
	}

	if ds, ok := lp.(*core.DataSource); ok {
		return []*core.DataSource{ds}
	}
	return nil
}

func buildEngineInfo(lp core.LogicalPlan, catalog *proto.Catalog, currentDb string, queryIssuer string, issuerAsParticipant bool) (*translator.EnginesInfo, error) {
	// construct catalog map
	catalogMap := make(map[string]*proto.TableEntry)
	for _, table := range catalog.GetTables() {
		tn := table.GetTableName()
		if _, exists := catalogMap[tn]; exists {
			return nil, fmt.Errorf("duplicate table exists in catalog")
		}
		catalogMap[tn] = table
	}

	party2Tables := make(map[string][]core.DbTable)
	tableToRefs := make(map[core.DbTable]core.DbTable)

	dsList := collectDataSourceNode(lp)
	if len(dsList) == 0 {
		return nil, fmt.Errorf("no data source in query")
	}

	for _, ds := range dsList {
		dbName := ds.DBName.String()
		tblName := ds.TableInfo().Name.String()

		if len(dbName) == 0 {
			dbName = currentDb
		}
		dbTable := core.NewDbTable(dbName, tblName)
		tn := dbTable.String()

		tblEntry, exists := catalogMap[tn]
		if !exists {
			return nil, fmt.Errorf("table `%s` not found in catalog", tn)
		}

		tblOwner := tblEntry.GetOwner().GetCode()
		party2Tables[tblOwner] = append(party2Tables[tblOwner], dbTable)

		// Note: ref table name empty means it is the same with itself
		refTblName := tblEntry.GetRefTable()
		if len(refTblName) == 0 {
			refTblName = tblEntry.GetTableName()
		}
		refDbTable, err := core.NewDbTableFromString(refTblName)
		if err != nil {
			return nil, fmt.Errorf("failed to create DbTable from %s: %+v", tblEntry.GetRefTable(), err)
		}
		dbType, err := core.ParseDBType(tblEntry.GetDbType())
		if err != nil {
			return nil, fmt.Errorf("unknown DBType of table %s： %v", tn, err)
		}

		refDbTable.SetDBType(dbType)

		tableToRefs[dbTable] = refDbTable
	}

	parties := make([]*translator.Participant, 0)
	for party := range party2Tables {
		parties = append(parties, &translator.Participant{
			PartyCode: party,
			// NOTE: For translator, other information (endpoint, token, pubkey...) is not important
			// TODO: remove unneeded fields
		})
	}

	if _, exists := party2Tables[queryIssuer]; !exists {
		if issuerAsParticipant {
			parties = append(parties, &translator.Participant{
				PartyCode: queryIssuer,
			})
		}
	}

	// sort parties by party code for deterministic in p2p
	sort.Slice(parties, func(i, j int) bool {
		return parties[i].PartyCode < parties[j].PartyCode
	})

	partyInfo := translator.NewPartyInfo(parties)

	engineInfo := translator.NewEnginesInfo(partyInfo, party2Tables)
	engineInfo.UpdateTableToRefs(tableToRefs)

	return engineInfo, nil
}
