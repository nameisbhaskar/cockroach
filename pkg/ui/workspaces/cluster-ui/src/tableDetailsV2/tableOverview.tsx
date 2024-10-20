// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

import { Icon } from "@cockroachlabs/ui-components";
import { Col, Row, Skeleton, Tooltip } from "antd";
import moment from "moment-timezone";
import React from "react";

import { useNodeStatuses } from "src/api";
import { TableDetailsResponse } from "src/api/databases/getTableMetadataApi";
import { PageSection } from "src/layouts";
import { SqlBox, SqlBoxSize } from "src/sql";
import { SummaryCard, SummaryCardItem } from "src/summaryCard";
import { Timestamp } from "src/timestamp";
import { StoreID } from "src/types/clusterTypes";
import { Bytes, DATE_WITH_SECONDS_FORMAT_24_TZ } from "src/util";

type TableOverviewProps = {
  tableDetails: TableDetailsResponse;
};

export const TableOverview: React.FC<TableOverviewProps> = ({
  tableDetails,
}) => {
  const { metadata } = tableDetails;
  const {
    nodeIDToRegion,
    storeIDToNodeID,
    isLoading: nodesLoading,
  } = useNodeStatuses();

  // getNodesByRegionDisplayStr returns a string that displays
  // the regions and nodes that the table is replicated across.
  const getNodesByRegionDisplayStr = (): string => {
    if (nodesLoading || !tableDetails?.metadata) {
      return "";
    }
    const nodesByRegion: Record<string, number[]> = {};
    metadata.store_ids.forEach(storeID => {
      const nodeID = storeIDToNodeID[storeID as StoreID];
      const region = nodeIDToRegion[nodeID];
      if (!nodesByRegion[region]) {
        nodesByRegion[region] = [];
      }
      nodesByRegion[region].push(nodeID);
    });
    return Object.entries(nodesByRegion)
      .map(
        ([region, nodes]) =>
          `${region} (${nodes.map(nid => "n" + nid).join(",")})`,
      )
      .join(", ");
  };

  const percentLiveDataWithPrecision = (
    metadata.percent_live_data * 100
  ).toFixed(2);

  const formattedErrorText = metadata.last_update_error
    ? "Update error: " + metadata.last_update_error
    : "";

  return (
    <>
      <PageSection>
        <SqlBox
          value={tableDetails.create_statement}
          size={SqlBoxSize.CUSTOM}
        />
      </PageSection>
      <PageSection>
        <Row justify={"end"}>
          <Col>
            <Tooltip title={formattedErrorText}>
              <Row gutter={8} align={"middle"} justify={"center"}>
                {metadata.last_update_error && (
                  <Icon fill="warning" iconName={"Caution"} />
                )}
                <Col>
                  Last updated:{" "}
                  <Timestamp
                    format={DATE_WITH_SECONDS_FORMAT_24_TZ}
                    time={moment.utc(metadata.last_updated)}
                    fallback={"Never"}
                  />
                </Col>
              </Row>
            </Tooltip>
          </Col>
        </Row>
        <Row gutter={8}>
          <Col span={12}>
            <SummaryCard>
              <SummaryCardItem
                label="Size"
                value={Bytes(metadata.replication_size_bytes)}
              />
              <SummaryCardItem label="Ranges" value={metadata.range_count} />
              <SummaryCardItem
                label="Regions / Nodes"
                value={
                  <Skeleton loading={nodesLoading}>
                    {getNodesByRegionDisplayStr()}
                  </Skeleton>
                }
              />
            </SummaryCard>
          </Col>
          <Col span={12}>
            <SummaryCard>
              <SummaryCardItem
                label="% of Live data"
                value={
                  <div>
                    <div>{percentLiveDataWithPrecision}% </div>
                    <div>
                      {Bytes(metadata.total_live_data_bytes)} /{" "}
                      {Bytes(metadata.total_live_data_bytes)}
                    </div>
                  </div>
                }
              />
              <SummaryCardItem
                label="Auto stats collections"
                value={metadata.auto_stats_enabled ? "Enabled" : "Disabled"}
              />
              <SummaryCardItem
                label="Stats last updated"
                value={
                  <Timestamp
                    time={moment.utc(metadata.stats_last_updated)}
                    format={DATE_WITH_SECONDS_FORMAT_24_TZ}
                    fallback={"Never"}
                  />
                }
              />
            </SummaryCard>
          </Col>
        </Row>
      </PageSection>
    </>
  );
};
