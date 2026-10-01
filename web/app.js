class LocationPicker {
  constructor(root) {
    if (!root) throw new Error("Location picker element is missing");

    this.root = root;
    this.input = root.querySelector(".location-input");
    this.hidden = root.querySelector("input[type=hidden]");
    this.options = root.querySelector(".location-options");
    this.locations = [];
    this.filtered = [];
    this.highlighted = -1;
    this.selected = null;

    this.input.addEventListener("focus", () => this.open());
    this.input.addEventListener("input", () => {
      this.selected = null;
      this.hidden.value = "";
      this.highlighted = -1;
      this.open();
    });
    this.input.addEventListener("keydown", (event) => this.handleKeydown(event));
    this.options.addEventListener("mousedown", (event) => {
      const target = event.target instanceof Element ? event.target : null;
      const option = target?.closest(".location-option");
      if (!option) return;
      event.preventDefault();
      this.selectById(option.dataset.locationId);
    });
    document.addEventListener("pointerdown", (event) => {
      if (!this.root.contains(event.target)) this.close();
    });
  }

  setLocations(values) {
    this.locations = values;
    this.input.disabled = false;
    this.render();
  }

  get value() {
    return this.hidden.value;
  }

  selectById(id) {
    const location = this.locations.find((candidate) => String(candidate.id) === String(id));
    if (location) this.select(location);
  }

  select(location) {
    this.selected = location;
    this.hidden.value = String(location.id);
    this.input.value = location.name;
    this.close();
  }

  open() {
    if (this.input.disabled) return;
    this.root.classList.add("open");
    this.input.setAttribute("aria-expanded", "true");
    this.render();
  }

  close() {
    this.root.classList.remove("open");
    this.input.setAttribute("aria-expanded", "false");
    this.input.removeAttribute("aria-activedescendant");
  }

  handleKeydown(event) {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      this.open();
      this.moveHighlight(1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      this.open();
      this.moveHighlight(-1);
    } else if (event.key === "Enter") {
      if (!this.root.classList.contains("open")) return;
      event.preventDefault();
      if (this.highlighted >= 0 && this.filtered[this.highlighted]) {
        this.select(this.filtered[this.highlighted]);
      } else if (this.filtered.length === 1) {
        this.select(this.filtered[0]);
      }
    } else if (event.key === "Escape") {
      this.close();
    }
  }

  moveHighlight(amount) {
    if (this.filtered.length === 0) return;
    if (this.highlighted < 0) {
      this.highlighted = amount > 0 ? 0 : this.filtered.length - 1;
    } else {
      this.highlighted = (this.highlighted + amount + this.filtered.length) % this.filtered.length;
    }
    this.render();
  }

  render() {
    const query = this.input.value.trim().toLowerCase();
    this.filtered = this.locations.filter((location) => location.name.toLowerCase().includes(query));
    this.options.replaceChildren();

    if (this.filtered.length === 0) {
      const empty = document.createElement("div");
      empty.className = "location-empty";
      empty.textContent = "No matching locations";
      this.options.append(empty);
      this.input.removeAttribute("aria-activedescendant");
      return;
    }

    const visibleLocations = this.filtered.slice(0, 100);
    if (this.highlighted >= visibleLocations.length) this.highlighted = -1;
    visibleLocations.forEach((location, index) => {
      const option = document.createElement("button");
      option.type = "button";
      option.className = "location-option";
      option.dataset.locationId = String(location.id);
      option.id = `${this.options.id}-option-${index}`;
      option.setAttribute("role", "option");
      option.setAttribute("aria-selected", String(this.selected?.id === location.id));
      option.textContent = location.name;
      if (index === this.highlighted) {
        option.classList.add("highlighted");
        this.input.setAttribute("aria-activedescendant", option.id);
      }
      this.options.append(option);
    });
  }
}

const fromPicker = new LocationPicker(document.querySelector("#from-picker"));
const toPicker = new LocationPicker(document.querySelector("#to-picker"));
const walkingMinimum = document.querySelector("#walking-minimum");
const walkingValue = document.querySelector("#walking-value");
const routeForm = document.querySelector("#route-form");
const findRouteButton = document.querySelector("#find-route");
const findRouteLabel = document.querySelector("#find-route-label");
const formMessage = document.querySelector("#form-message");
const routeTitle = document.querySelector("#route-title");
const routeContent = document.querySelector("#route-content");

let locations = [];

walkingMinimum.addEventListener("input", () => {
  walkingValue.textContent = `${walkingMinimum.value} minutes`;
});

routeForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!validateLocations()) return;
  await showRoute();
});

async function loadLocations() {
  try {
    const response = await fetch("/api/locations");
    const data = await readJSON(response);
    if (!response.ok) throw new Error(data.error || "Could not load locations");
    if (!Array.isArray(data)) throw new Error("The locations response was invalid");

    locations = data.filter((location) => location && location.id !== undefined && typeof location.name === "string");
    if (locations.length === 0) throw new Error("No locations are available");

    fromPicker.setLocations(locations);
    toPicker.setLocations(locations);
    if (locations.length > 1) {
      fromPicker.select(locations[0]);
      toPicker.select(locations[locations.length - 1]);
    } else {
      fromPicker.select(locations[0]);
      toPicker.select(locations[0]);
    }
    findRouteButton.disabled = false;
  } catch (error) {
    setMessage(errorMessage(error, "Could not load locations"), true);
    findRouteButton.disabled = true;
  }
}

function validateLocations() {
  if (fromPicker.value && toPicker.value) return true;
  setMessage("Choose a starting location and destination from the search results.", true);
  return false;
}

async function showRoute() {
  const selectedMinimum = Number(walkingMinimum.value);
  setMessage("");
  setBusy(true, "Finding route...");

  try {
    const route = await requestRoute(selectedMinimum);
    renderRoute(route, selectedMinimum);
  } catch (error) {
    routeTitle.textContent = "No route found";
    routeContent.replaceChildren();
    routeContent.className = "empty-state error-state";
    appendText(routeContent, errorMessage(error, "Route calculation failed"));
    setMessage(errorMessage(error, "Route calculation failed"), true);
  } finally {
    setBusy(false);
  }
}

async function requestRoute(minimumWalkingMinutes = Number(walkingMinimum.value)) {
  if (!validateLocations()) throw new Error("Choose valid locations first");

  const response = await fetch("/api/route", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      from: Number(fromPicker.value),
      to: Number(toPicker.value),
      minWalkingMinutes: minimumWalkingMinutes,
    }),
  });
  const data = await readJSON(response);
  if (!response.ok) throw new Error(data.error || "Route calculation failed");
  return data;
}

async function readJSON(response) {
  const text = await response.text();
  if (!text) return {};
  try {
    return JSON.parse(text);
  } catch {
    throw new Error("The server returned an invalid response");
  }
}

function renderRoute(route, minimumWalkingMinutes = Number(walkingMinimum.value)) {
  const destination = locations.find((location) => String(location.id) === toPicker.value);
  routeTitle.textContent = destination ? `To ${destination.name}` : "Route found";
  routeContent.replaceChildren();
  routeContent.className = "route-result";

  const stats = document.createElement("div");
  stats.className = "stats-grid";
  addStat(stats, formatNumber(route.totalMinutes), "total minutes");
  addStat(stats, formatNumber(route.walkingMinutes), "walking minutes");
  addStat(stats, formatNumber(route.walkingMeters), "walking metres");
  addStat(stats, String(route.transfers), "transfers");
  addStat(stats, String(route.nodesExplored), "A* states explored");
  routeContent.append(stats);

  const constraint = document.createElement("p");
  constraint.className = "constraint-note";
  constraint.textContent = `Fastest route with at least ${minimumWalkingMinutes} minutes walking`;
  routeContent.append(constraint);

  const steps = document.createElement("ol");
  steps.className = "steps-list";
  for (const step of collapseRouteSteps(route.steps)) {
    const item = document.createElement("li");
    const icon = document.createElement("span");
    icon.className = `step-icon ${step.mode}`;
    icon.textContent = modeIcon(step.mode);
    icon.setAttribute("aria-hidden", "true");

    const details = document.createElement("div");
    details.className = "step-details";
    const title = document.createElement("strong");
    title.textContent = stepTitle(step);
    const description = document.createElement("span");
    description.textContent = `${formatNumber(step.durationMinutes)} min / ${formatNumber(step.distanceMeters)} m`;
    details.append(title, description);
    item.append(icon, details);
    steps.append(item);
  }
  routeContent.append(steps);
}

function stepTitle(step) {
  const destination = locationLabel(step.to);
  if (step.mode === "walk") {
    const transferStation = sameStation(step.from, step.to);
    if (transferStation) return `Transfer at ${transferStation}`;
    return destination === "the next stop" ? "Continue walking" : `Walk to ${destination}`;
  }
  const mode = step.mode === "train" ? "Train" : "Bus";
  return `Take ${step.service || mode} to ${destination}`;
}

function sameStation(from, to) {
  const fromStation = stationName(from?.name);
  const toStation = stationName(to?.name);
  return fromStation && fromStation === toStation ? fromStation : null;
}

function stationName(name) {
  const match = typeof name === "string" ? name.match(/^(.*? Station)(?:, Platform\b.*)?$/) : null;
  return match?.[1] || null;
}

function collapseRouteSteps(routeSteps) {
  const collapsed = [];
  for (const step of routeSteps) {
    const previous = collapsed[collapsed.length - 1];
    const sameWalkingLeg = previous?.mode === "walk" && step.mode === "walk";
    const sameTransitLeg = previous?.mode !== "walk"
      && step.mode !== "walk"
      && previous?.mode === step.mode
      && previous?.service === step.service;
    if (sameWalkingLeg || sameTransitLeg) {
      previous.to = step.to;
      previous.durationMinutes += step.durationMinutes;
      previous.distanceMeters += step.distanceMeters;
      continue;
    }
    collapsed.push({ ...step });
  }
  return collapsed;
}

function locationLabel(location) {
  if (String(location?.id) === toPicker.value) {
    const destination = locations.find((candidate) => String(candidate.id) === toPicker.value);
    return destination?.name || "your destination";
  }
  if (location?.name && location.named !== false && !/^Walking node \d+$/.test(location.name)) {
    return location.name;
  }
  return "the next stop";
}

function modeIcon(mode) {
  if (mode === "walk") return ">";
  if (mode === "train") return "[T]";
  return "[B]";
}

function addStat(container, value, label) {
  const stat = document.createElement("div");
  stat.className = "stat";
  const number = document.createElement("strong");
  number.textContent = value;
  const caption = document.createElement("span");
  caption.textContent = label;
  stat.append(number, caption);
  container.append(stat);
}

function appendText(element, text) {
  const paragraph = document.createElement("p");
  paragraph.textContent = text;
  element.append(paragraph);
}

function setBusy(busy, label) {
  findRouteButton.disabled = busy;
  findRouteLabel.textContent = busy ? label : "Find route";
}

function setMessage(message, isError = false) {
  formMessage.textContent = message;
  formMessage.className = isError ? "form-message visible error-text" : "form-message";
}

function errorMessage(error, fallback) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function formatNumber(value) {
  return new Intl.NumberFormat("en-AU", { maximumFractionDigits: 1 }).format(Number(value) || 0);
}

loadLocations();
