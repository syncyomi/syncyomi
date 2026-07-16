import { formatISO9075 } from "date-fns";

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
  if (import.meta.env.DEV) return "http://localhost:8282/";

  return `${window.location.origin}${baseUrl()}`;
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
