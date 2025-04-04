import React, { ChangeEvent } from 'react';
import { InlineField, Input, Stack } from '@grafana/ui';
import { QueryEditorProps } from '@grafana/data';
import { DataSource } from '../datasource';
import { MyDataSourceOptions, MyQuery } from '../types';

type Props = QueryEditorProps<DataSource, MyQuery, MyDataSourceOptions>;

export function QueryEditor({ query, onChange, onRunQuery }: Props) {
  const onMetricChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, metric: event.target.value });
    onRunQuery(); // execute query on metric change
  };

  const onQueryTextChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, queryText: event.target.value });
  };

  const onConstantChange = (event: ChangeEvent<HTMLInputElement>) => {
    onChange({ ...query, constant: parseFloat(event.target.value) });
    onRunQuery();
  };

  const { metric, queryText, constant } = query;

  return (
    <Stack gap={0}>
      <InlineField label="Metric" tooltip="Enter a Prometheus metric like go_threads or go_gc_duration_seconds">
        <Input
          id="query-editor-metric"
          onChange={onMetricChange}
          value={metric || ''}
          placeholder="e.g. go_threads"
          width={40}
        />
      </InlineField>

      <InlineField label="Constant">
        <Input
          id="query-editor-constant"
          onChange={onConstantChange}
          value={constant}
          width={8}
          type="number"
          step="0.1"
        />
      </InlineField>

      <InlineField label="Query Text" labelWidth={16} tooltip="Not used yet">
        <Input
          id="query-editor-query-text"
          onChange={onQueryTextChange}
          value={queryText || ''}
          required
          placeholder="Enter a query"
        />
      </InlineField>
    </Stack>
  );
}

