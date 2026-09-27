<script lang="ts">
  import { onMount } from 'svelte';
  import { money } from '$lib/format';
  import {
    Chart,
    registerables,
    type ChartDataset,
    type TooltipModel,
    type TooltipItem
  } from 'chart.js';
  Chart.register(...registerables);
  let {
    labels,
    datasets,
    type = 'bar',
    title,
    currency = 'USD'
  } = $props<{
    labels: string[];
    datasets: ChartDataset<'bar' | 'line', number[]>[];
    type?: 'bar' | 'line';
    title: string;
    currency?: string;
  }>();
  let canvas: HTMLCanvasElement;
  let container: HTMLDivElement;
  let chart: Chart<'bar' | 'line', number[]> | undefined;
  let tooltip = $state<{
    title: string;
    x: number;
    y: number;
    rows: { label: string; value: string; color: string }[];
  } | null>(null);
  let activeIndex = 0;
  onMount(() => {
    chart = new Chart(canvas, {
      type,
      data: { labels, datasets },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { intersect: false, mode: 'index' },
        plugins: {
          legend: { display: false },
          tooltip: {
            enabled: false,
            external: ({
              tooltip: model
            }: {
              tooltip: TooltipModel<'bar' | 'line'>;
            }) => {
              if (!model.opacity) {
                tooltip = null;
                return;
              }
              tooltip = {
                title: model.title?.[0] || '',
                x: Math.max(
                  10,
                  Math.min(
                    container.clientWidth - 226,
                    canvas.offsetLeft + model.caretX + 14
                  )
                ),
                y: Math.max(
                  8,
                  Math.min(
                    container.clientHeight - 112,
                    canvas.offsetTop + model.caretY - 35
                  )
                ),
                rows: model.dataPoints.map(
                  (point: TooltipItem<'bar' | 'line'>) => ({
                    label: point.dataset.label || '',
                    value: money(String(point.raw), currency),
                    color: String(
                      point.dataset.borderColor ||
                        point.dataset.backgroundColor ||
                        '#7774ce'
                    )
                  })
                )
              };
            }
          }
        },
        scales: {
          x: {
            grid: { display: false },
            border: { display: false },
            ticks: {
              color: '#81858e',
              font: { size: 10 },
              maxRotation: 0,
              maxTicksLimit: 8
            }
          },
          y: {
            border: { display: false },
            grid: { color: 'rgba(128,128,145,.10)' },
            ticks: {
              color: '#81858e',
              font: { size: 10 },
              maxTicksLimit: 5,
              callback: (v: string | number) =>
                Number(v).toLocaleString('en-US', { notation: 'compact' })
            }
          }
        }
      }
    });
    return () => chart?.destroy();
  });
  $effect(() => {
    const data = { labels, datasets };
    if (chart) {
      if ('type' in chart.config) chart.config.type = type;
      chart.data = data;
      chart.update();
      tooltip = null;
    }
  });
  function activate(index: number) {
    if (!chart || !labels.length) return;
    activeIndex = Math.max(0, Math.min(labels.length - 1, index));
    const points = datasets.map(
      (_: ChartDataset<'bar' | 'line', number[]>, datasetIndex: number) => ({
        datasetIndex,
        index: activeIndex
      })
    );
    const element = chart.getDatasetMeta(0).data[activeIndex];
    if (!element) return;
    chart.setActiveElements(points);
    chart.tooltip?.setActiveElements(points, { x: element.x, y: element.y });
    chart.update();
  }
  function dismiss() {
    chart?.setActiveElements([]);
    chart?.tooltip?.setActiveElements([], { x: 0, y: 0 });
    chart?.update();
    tooltip = null;
  }
  function keydown(event: KeyboardEvent) {
    if (['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) {
      event.preventDefault();
      activate(
        event.key === 'Home'
          ? 0
          : event.key === 'End'
            ? labels.length - 1
            : activeIndex + (event.key === 'ArrowRight' ? 1 : -1)
      );
    } else if (event.key === 'Escape') {
      dismiss();
    }
  }
</script>

<div class="chart-canvas" bind:this={container}>
  <canvas
    bind:this={canvas}
    aria-label={`${title}. Use left and right arrow keys to inspect monthly values.`}
    tabindex="0"
    onfocus={() => activate(0)}
    onblur={dismiss}
    onkeydown={keydown}
  ></canvas>
  {#if tooltip}<div
      class="chart-tooltip"
      role="tooltip"
      style={`left:${tooltip.x}px;top:${tooltip.y}px`}
    >
      <div class="chart-tooltip-title">
        {tooltip.title}<span>{currency}</span>
      </div>
      {#each tooltip.rows as row}<div class="chart-tooltip-row">
          <i style={`background:${row.color}`}></i><span>{row.label}</span
          ><strong>{row.value}</strong>
        </div>{/each}
    </div>{/if}
</div>
