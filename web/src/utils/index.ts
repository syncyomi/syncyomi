import { formatDistanceToNowStrict, formatISO9075 } from "date-fns";

// sleep for x ms
export function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// get baseUrl sent from server rendered index template
export function baseUrl() {
  let baseUrl = "";
  if (window.APP.baseUrl) {
    if (window.APP.baseUrl === "{{.BaseUrl}}") {
      baseUrl = "/"; // Use / as default
    } else {
      baseUrl = window.APP.baseUrl;
    }
  }
  return baseUrl;
}

// get sseBaseUrl for SSE
export function sseBaseUrl() {
  if (process.env.NODE_ENV === "development") return "http://localhost:8282/";

  return `${window.location.origin}${baseUrl()}`;
}

export function classNames(...classes: string[]) {
  return classes.filter(Boolean).join(" ");
}

// simplify date
export function simplifyDate(date: string) {
  if (date === "") {
    return "n/a";
  } else if (date !== "0001-01-01T00:00:00Z") {
    return formatISO9075(new Date(date));
  }

  return "n/a";
}

// if empty date show as n/a
export function IsEmptyDate(date: string) {
  if (date !== "0001-01-01T00:00:00Z") {
    return formatDistanceToNowStrict(new Date(date), { addSuffix: true });
  }
  return "n/a";
}
