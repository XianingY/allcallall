function normalizeBaseURL(value) {
  return String(value || "http://localhost:5173").replace(/\/+$/, "");
}

function createRouteHelpers(baseURL) {
  const webAppURL = normalizeBaseURL(baseURL);
  const webAppOrigin = new URL(webAppURL).origin;

  function routeURL(route) {
    const normalizedRoute = route.startsWith("/") ? route : `/${route}`;
    return `${webAppURL}${normalizedRoute}`;
  }

  function isInternalWebURL(target) {
    try {
      const parsed = new URL(target);
      return parsed.origin === webAppOrigin && target.startsWith(webAppURL);
    } catch {
      return false;
    }
  }

  function normalizeRouteTarget(target) {
    if (!target || typeof target !== "string") {
      return null;
    }
    if (target.startsWith("allcallall://rooms/")) {
      return routeURL(`/meetings/${target.replace("allcallall://rooms/", "").split(/[?#]/)[0]}`);
    }
    if (target.startsWith("allcallall://conversations/")) {
      return routeURL(`/conversations/${target.replace("allcallall://conversations/", "").split(/[?#]/)[0]}`);
    }
    if (target === "allcallall://meetings" || target.startsWith("allcallall://meetings?")) {
      return routeURL("/meetings");
    }
    if (target.startsWith("/rooms/")) {
      return routeURL(target.replace(/^\/rooms\//, "/meetings/"));
    }
    if (target.startsWith("/meetings/") || target.startsWith("/conversations/") || target === "/meetings") {
      return routeURL(target);
    }
    if (isInternalWebURL(target)) {
      return target;
    }
    return null;
  }

  return {
    webAppURL,
    webAppOrigin,
    routeURL,
    isInternalWebURL,
    normalizeRouteTarget,
  };
}

module.exports = {
  createRouteHelpers,
  normalizeBaseURL,
};
