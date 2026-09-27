import Decimal from 'decimal.js';
export function money(value: string | number | undefined, currency = 'USD') {
  const number = new Decimal(value || 0);
  // Decimal rounding happens before display; floats are only used by charts.
  const isFiat = Intl.supportedValuesOf('currency').includes(currency);
  const digits = isFiat
    ? new Intl.NumberFormat('en', {
        style: 'currency',
        currency
      }).resolvedOptions().maximumFractionDigits
    : Math.max(2, number.decimalPlaces());
  const fixed = number.toFixed(digits ?? 2);
  const [integer, fraction] = fixed.split('.');
  const grouped = integer.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  const symbol: Record<string, string> = { USD: '$', EUR: '€', GBP: '£' };
  return symbol[currency]
    ? `${number.isNegative() ? '−' : ''}${symbol[currency]}${grouped.replace('-', '')}.${fraction}`
    : `${grouped}${fraction ? '.' + fraction : ''} ${currency}`;
}
export function dateLabel(value: string) {
  return value
    ? new Date(value.slice(0, 10) + 'T12:00:00Z').toLocaleDateString('en-US', {
        month: 'short',
        day: 'numeric',
        year: 'numeric',
        timeZone: 'UTC'
      })
    : '—';
}
export function shortAccount(value: string) {
  return value.split(':').slice(1).join(' / ') || value;
}
export function download(name: string, text: string, type = 'text/plain') {
  const url = URL.createObjectURL(new Blob([text], { type }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export const csv = (value: string) => `"${value.replaceAll('"', '""')}"`;
