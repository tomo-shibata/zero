import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import type { PointerEvent as ReactPointerEvent } from 'react'
import type { StockHistory } from '../../../entities/stock-history/model'
import { useElementSize } from '../hooks/useElementSize'
import { formatAxisPrice, priceTicks } from '../lib/axisTicks'
import { dateAtPosition, dateTicks, makeScales } from '../lib/chartGeometry'
import type { PlotBox } from '../lib/chartGeometry'
import { buildChartModel } from '../lib/chartModel'
import type { ChartMarker } from '../lib/chartModel'
import { nearestClose } from '../lib/nearestClose'
import { closeTooltip, tradeTooltipLines } from '../lib/tooltipLines'
import { placeTooltip } from '../lib/tooltipPlacement'
import './PriceChart.css'

const NO_PRICES_MESSAGE = 'この期間の価格はまだありません'
const LEGEND = '▲ 買い ▼ 売り'

/** 測れていない間（最初の描画と、ResizeObserver のない環境）の大きさ（px）。 */
const DEFAULT_SIZE = { width: 640, height: 320 }

/** 描く範囲の周りの余白（px）。左は縦軸の文字の幅に合わせて決める。 */
const MARGIN = { top: 12, right: 20, bottom: 28 }
const AXIS_LABEL_CHAR_WIDTH_PX = 7.5
const AXIS_LABEL_GAP_PX = 14
/** 縦軸の目盛りの文字が重ならないための、目盛り1つあたりに取る高さ（px）。 */
const TICK_SPACING_PX = 24
/** 縦軸の目盛りの数の下限・上限。 */
const MIN_Y_TICKS = 2
const MAX_Y_TICKS = 6
/** 縦軸の文字と描く範囲の左端の間（px）。 */
const Y_LABEL_OFFSET_PX = 8
/** 横軸の文字の、描く範囲の下端からの距離（px）。 */
const X_LABEL_OFFSET_PX = 18

/** 印（三角形）の半分の大きさ（px）。 */
const MARKER_HALF_PX = 6
/** 指している線の上の点の半径（px）。 */
const HOVER_DOT_RADIUS_PX = 4
/** 終値が1件だけのときに描く点の半径（px）。 */
const SINGLE_POINT_RADIUS_PX = 4

/**
 * 吹き出し。anchor は指す点（チャートの左上からの px）。吹き出しは、その点から離して、チャートの箱の中に収まるように置く（tooltipPlacement）。
 * boxWidth・boxHeight は、点を決めたときのチャートの大きさ（大きさが変わったら、点の位置が合わなくなるので消す）。
 * onLine は、線の上の点を指しているか（そうなら、その点に小さな丸を描く）。
 */
type Tooltip = { lines: string[]; anchorX: number; anchorY: number; boxWidth: number; boxHeight: number; onLine: boolean }

type PriceChartProps = {
  /** 通貨・銘柄名は history.stock から取る。 */
  history: StockHistory
}

/**
 * 価格のチャート（要件6〜要件13）。終値の線・売買の印・軸・凡例を SVG で描き、線と印の吹き出しを出す。
 * 描く値は buildChartModel（分割で割った値）。吹き出しの文字は tooltipLines（API が丸めた価格）。
 * 吹き出しは、マウスは乗せている間、タップは次にチャートの外を押すまで出す（要件10・要件11）。
 */
export function PriceChart({ history }: PriceChartProps) {
  const { code, name, currency } = history.stock
  const model = useMemo(() => buildChartModel(history), [history])
  const { ref: canvasRef, size } = useElementSize<HTMLDivElement>()
  const [tooltip, setTooltip] = useState<Tooltip | null>(null)

  // 箱の実際の大きさで描く（SVG の座標 1 が CSS の 1px にあたる。拡大・縮小して描くと、吹き出し（CSS の px）の位置が点からずれる）。
  const width = Math.max(size?.width ?? DEFAULT_SIZE.width, 1)
  const height = Math.max(size?.height ?? DEFAULT_SIZE.height, 1)
  const plotHeight = Math.max(height - MARGIN.top - MARGIN.bottom, 1)
  // 縦軸の目盛りの数は、描く範囲の高さに合わせる（低いチャートで文字が重ならないよう、24px に1つ、2〜6個）。
  const maxTicks = Math.max(MIN_Y_TICKS, Math.min(MAX_Y_TICKS, Math.floor(plotHeight / TICK_SPACING_PX)))
  const yTickLabels = useMemo(
    () =>
      priceTicks(currency, model.yRange.min, model.yRange.max, maxTicks).map((value) => ({
        value,
        label: formatAxisPrice(currency, value),
      })),
    [currency, model.yRange, maxTicks],
  )
  const longestLabel = Math.max(...yTickLabels.map((tick) => tick.label.length))
  const left = Math.ceil(longestLabel * AXIS_LABEL_CHAR_WIDTH_PX + AXIS_LABEL_GAP_PX)
  const box: PlotBox = {
    left,
    top: MARGIN.top,
    width: Math.max(width - left - MARGIN.right, 1),
    height: plotHeight,
  }
  const scales = makeScales(model, box)
  const xTicks = dateTicks(model.xRange, box.width)

  // 箱の大きさが変わったら（画面の向きの変更など）、吹き出しを消す（指す点の位置が合わなくなるため）。描画の途中で state を直す書き方（React の案内どおり）。
  if (tooltip !== null && (tooltip.boxWidth !== width || tooltip.boxHeight !== height)) setTooltip(null)

  // 吹き出しの大きさを測って、チャートの箱の中に収まる位置に置く（描画の後、画面に出る前に）。
  // 先に左上（0, 0）に戻してから測る（前の位置のままだと、箱の右端までの残りで幅が縮み、実際より小さく測れるため）。
  const tooltipRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const element = tooltipRef.current
    if (element === null || tooltip === null) return
    element.style.left = '0px'
    element.style.top = '0px'
    const { width: tooltipWidth, height: tooltipHeight } = element.getBoundingClientRect()
    const placed = placeTooltip({
      anchorX: tooltip.anchorX,
      anchorY: tooltip.anchorY,
      width: tooltipWidth,
      height: tooltipHeight,
      boxWidth: tooltip.boxWidth,
      boxHeight: tooltip.boxHeight,
    })
    element.style.left = `${placed.left}px`
    element.style.top = `${placed.top}px`
  }, [tooltip])

  // タップで出した吹き出しは、チャートの外を押すと消す（マウスの吹き出しは乗せている間だけなので、これは主にタッチ用）。
  const hasTooltip = tooltip !== null
  useEffect(() => {
    if (!hasTooltip) return
    const hideOutside = (event: PointerEvent) => {
      if (!(event.target instanceof Node) || !canvasRef.current?.contains(event.target)) setTooltip(null)
    }
    document.addEventListener('pointerdown', hideOutside)
    return () => document.removeEventListener('pointerdown', hideOutside)
  }, [hasTooltip, canvasRef])

  /** 線の吹き出し: ポインターの位置を日付に換えて、いちばん近い終値を出す。 */
  const showNearestClose = (event: ReactPointerEvent<SVGRectElement>) => {
    const date = dateAtPosition(model.xRange, event.clientX, event.currentTarget.getBoundingClientRect())
    const point = date === null ? null : nearestClose(model.line, date)
    if (point === null) return
    setTooltip({
      lines: [closeTooltip(currency, point.close)],
      anchorX: scales.x(point.date),
      anchorY: scales.y(point.value),
      boxWidth: width,
      boxHeight: height,
      onLine: true,
    })
  }
  /** 印の吹き出し: その印と同じ取引日の売買を、印の順（表と同じ順）に出す。 */
  const showTradesOfDay = (marker: ChartMarker) => {
    const trades = model.markers.filter((candidate) => candidate.date === marker.date).map((candidate) => candidate.trade)
    setTooltip({
      lines: tradeTooltipLines(currency, trades),
      anchorX: scales.x(marker.date),
      anchorY: scales.y(marker.value),
      boxWidth: width,
      boxHeight: height,
      onLine: false,
    })
  }
  // 乗せて出した吹き出しは、ポインターが外れたら消す（マウスもペンも）。タッチでは、指を離した直後にも pointerleave が来るので消さない。
  const hideUnlessTouch = (event: ReactPointerEvent) => {
    if (event.pointerType !== 'touch') setTooltip(null)
  }

  return (
    <div className="price-chart">
      <div ref={canvasRef} className="price-chart__canvas">
        <svg
          role="img"
          aria-label={`${code} ${name}の価格のチャート`}
          className="price-chart__svg"
          viewBox={`0 0 ${width} ${height}`}
        >
          {yTickLabels.map((tick) => {
            const y = scales.y(tick.value)
            return (
              <g key={tick.value}>
                <line className="price-chart__grid" x1={box.left} x2={box.left + box.width} y1={y} y2={y} />
                <text
                  data-chart-part="y-tick"
                  className="price-chart__axis-label"
                  x={box.left - Y_LABEL_OFFSET_PX}
                  y={y}
                  textAnchor="end"
                  dominantBaseline="middle"
                >
                  {tick.label}
                </text>
              </g>
            )
          })}
          {xTicks.map((date, index) => (
            <text
              key={date}
              data-chart-part="x-tick"
              className="price-chart__axis-label"
              x={scales.x(date)}
              y={box.top + box.height + X_LABEL_OFFSET_PX}
              textAnchor={index === 0 ? 'start' : index === xTicks.length - 1 ? 'end' : 'middle'}
            >
              {date}
            </text>
          ))}
          {model.line.length >= 2 && (
            <path
              data-chart-part="line"
              className="price-chart__line"
              d={`M${model.line.map((point) => `${scales.x(point.date).toFixed(2)},${scales.y(point.value).toFixed(2)}`).join('L')}`}
            />
          )}
          {model.line.length === 1 && (
            <circle
              data-chart-part="point"
              className="price-chart__point"
              cx={scales.x(model.line[0].date)}
              cy={scales.y(model.line[0].value)}
              r={SINGLE_POINT_RADIUS_PX}
            />
          )}
          {/* 線の吹き出しを受け取る領域（描く範囲を覆う）。印はこの上に描くので、印の上は印が受け取る。 */}
          <rect
            data-chart-part="plot-area"
            className="price-chart__plot-area"
            x={box.left}
            y={box.top}
            width={box.width}
            height={box.height}
            onPointerMove={showNearestClose}
            onPointerDown={showNearestClose}
            onPointerLeave={hideUnlessTouch}
          />
          {model.markers.map((marker, index) => {
            const x = scales.x(marker.date)
            const y = scales.y(marker.value)
            const half = MARKER_HALF_PX
            // 買いは上向き、売りは下向きの三角形（色だけで区別しない。要件8）。
            const points =
              marker.side === 'buy'
                ? `${x},${y - half} ${x - half},${y + half} ${x + half},${y + half}`
                : `${x},${y + half} ${x - half},${y - half} ${x + half},${y - half}`
            return (
              <polygon
                key={index}
                data-chart-part="marker"
                data-side={marker.side}
                data-date={marker.date}
                className={`price-chart__marker price-chart__marker--${marker.side}`}
                points={points}
                onPointerEnter={(event) => {
                  // タッチは、押したときの pointerdown で出す（pointerenter と二重にならないように）。マウスもペン（浮かせた）も乗せたら出す。
                  if (event.pointerType !== 'touch') showTradesOfDay(marker)
                }}
                onPointerDown={() => showTradesOfDay(marker)}
                onPointerLeave={hideUnlessTouch}
              />
            )
          })}
          {tooltip?.onLine && (
            <circle
              data-chart-part="hover-dot"
              className="price-chart__hover-dot"
              cx={tooltip.anchorX}
              cy={tooltip.anchorY}
              r={HOVER_DOT_RADIUS_PX}
            />
          )}
        </svg>
        {model.hasNoPrices && <p className="price-chart__empty">{NO_PRICES_MESSAGE}</p>}
        {/* 位置（left・top）は、上の useLayoutEffect が、測った大きさから決めて付ける。 */}
        {tooltip !== null && (
          <div ref={tooltipRef} role="tooltip" className="price-chart__tooltip">
            {tooltip.lines.map((line, index) => (
              <div key={index}>{line}</div>
            ))}
          </div>
        )}
      </div>
      {/* 三角形に印と同じ色を付けるため、色付きの見た目と、画面に出さない読み上げ用の同じ文に分ける。 */}
      <p className="price-chart__legend">
        <span aria-hidden="true">
          <span className="price-chart__legend-buy">▲</span> 買い <span className="price-chart__legend-sell">▼</span> 売り
        </span>
        <span className="visually-hidden">{LEGEND}</span>
      </p>
    </div>
  )
}
