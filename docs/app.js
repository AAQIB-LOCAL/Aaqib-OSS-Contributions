(() => {
  "use strict";

  const state = {
    items: [],
    repositoryCounts: new Map(),
    page: 1,
    pageSize: 25,
    query: "",
    repository: "",
    sort: "repository"
  };

  const elements = {
    merged: document.getElementById("merged-count"),
    repositories: document.getElementById("repository-count"),
    authored: document.getElementById("authored-count"),
    average: document.getElementById("average-count"),
    topRepositories: document.getElementById("top-repositories"),
    search: document.getElementById("search-input"),
    repository: document.getElementById("repository-filter"),
    sort: document.getElementById("sort-select"),
    pageSize: document.getElementById("page-size"),
    rows: document.getElementById("contribution-rows"),
    resultCount: document.getElementById("result-count"),
    paginationSummary: document.getElementById("pagination-summary"),
    pageIndicator: document.getElementById("page-indicator"),
    previous: document.getElementById("previous-page"),
    next: document.getElementById("next-page"),
    empty: document.getElementById("empty-state"),
    themeToggle: document.getElementById("theme-toggle")
  };

  const numberFormat = new Intl.NumberFormat("en");

  function formatNumber(value) {
    return numberFormat.format(Number(value) || 0);
  }

  function createElement(tag, className, text) {
    const node = document.createElement(tag);
    if (className) node.className = className;
    if (text !== undefined) node.textContent = String(text);
    return node;
  }

  function pullRequestURL(item) {
    return repositoryURL(item.repository) +
      "/pull/" + encodeURIComponent(String(item.number));
  }

  function repositoryURL(repository) {
    return "https://github.com/" + repository.split("/").map(encodeURIComponent).join("/");
  }

  function normaliseContribution(record) {
    if (!record || typeof record.repository !== "string" ||
        !/^[^/]+\/[^/]+$/.test(record.repository) ||
        !Number.isSafeInteger(Number(record.number)) || Number(record.number) < 1) {
      return null;
    }

    return {
      repository: record.repository,
      number: Number(record.number),
      description: typeof record.description === "string" ? record.description : "",
      status: "Merged"
    };
  }

  function renderMetrics(ledger) {
    const repositoryCount = state.repositoryCounts.size;
    const mergedCount = state.items.length;
    const authoredCount = Number(ledger.authoredPullRequestHighWatermark) || 0;

    elements.merged.textContent = formatNumber(mergedCount);
    elements.repositories.textContent = formatNumber(repositoryCount);
    elements.authored.textContent = formatNumber(authoredCount);
    elements.average.textContent = repositoryCount
      ? (mergedCount / repositoryCount).toFixed(1)
      : "0.0";
  }

  function populateRepositoryFilter() {
    const repositories = Array.from(state.repositoryCounts.keys())
      .sort((left, right) => left.localeCompare(right));

    const fragment = document.createDocumentFragment();
    fragment.append(createElement("option", "", "All repositories"));
    repositories.forEach((repository) => {
      const option = createElement("option", "", repository);
      option.value = repository;
      fragment.append(option);
    });
    elements.repository.replaceChildren(fragment);
  }

  function renderTopRepositories() {
    const top = Array.from(state.repositoryCounts.entries())
      .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))
      .slice(0, 7);
    const maxCount = top.length ? top[0][1] : 1;
    const fragment = document.createDocumentFragment();

    top.forEach(([repository, count]) => {
      const row = createElement("div", "repository-bar-row");
      const link = createElement("a", "repository-bar-name", repository);
      link.href = repositoryURL(repository);
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.title = repository;

      const track = createElement("div", "repository-bar-track");
      track.setAttribute("aria-label", formatNumber(count) + " merged pull requests");
      const fill = createElement("div", "repository-bar-fill");
      fill.style.width = Math.max(4, (count / maxCount) * 100) + "%";
      track.append(fill);

      row.append(link, track, createElement("span", "repository-bar-value", count));
      fragment.append(row);
    });

    elements.topRepositories.replaceChildren(fragment);
  }

  function filteredContributions() {
    const query = state.query.toLocaleLowerCase();
    const filtered = state.items.filter((item) => {
      if (state.repository && item.repository !== state.repository) return false;
      if (!query) return true;
      const searchable = [
        item.repository,
        String(item.number),
        "#" + item.number,
        item.description
      ].join(" ").toLocaleLowerCase();
      return searchable.includes(query);
    });

    const repoCounts = state.repositoryCounts;
    filtered.sort((left, right) => {
      if (state.sort === "pr-desc") {
        return right.number - left.number || left.repository.localeCompare(right.repository);
      }
      if (state.sort === "pr-asc") {
        return left.number - right.number || left.repository.localeCompare(right.repository);
      }
      if (state.sort === "repo-count") {
        return (repoCounts.get(right.repository) - repoCounts.get(left.repository)) ||
          left.repository.localeCompare(right.repository) ||
          right.number - left.number;
      }
      return left.repository.localeCompare(right.repository) || right.number - left.number;
    });
    return filtered;
  }

  function renderRows() {
    const filtered = filteredContributions();
    const pageCount = Math.max(1, Math.ceil(filtered.length / state.pageSize));
    state.page = Math.min(state.page, pageCount);
    const startIndex = (state.page - 1) * state.pageSize;
    const pageItems = filtered.slice(startIndex, startIndex + state.pageSize);
    const fragment = document.createDocumentFragment();

    pageItems.forEach((item) => {
      const row = document.createElement("tr");

      const repositoryCell = createElement("td", "repository-cell");
      const repositoryLink = createElement("a", "", item.repository);
      repositoryLink.href = repositoryURL(item.repository);
      repositoryLink.target = "_blank";
      repositoryLink.rel = "noopener noreferrer";
      repositoryCell.append(repositoryLink);

      const prCell = createElement("td", "pr-cell");
      const prLink = createElement("a", "", "#" + item.number);
      prLink.href = pullRequestURL(item);
      prLink.target = "_blank";
      prLink.rel = "noopener noreferrer";
      prCell.append(prLink);

      const descriptionCell = createElement("td", "description-cell", item.description || "No description provided");
      const statusCell = document.createElement("td");
      statusCell.append(createElement("span", "status-pill", item.status));

      row.append(repositoryCell, prCell, descriptionCell, statusCell);
      fragment.append(row);
    });

    if (pageItems.length === 0) {
      const row = document.createElement("tr");
      const cell = createElement("td", "loading-cell", "No matching contributions.");
      cell.colSpan = 4;
      row.append(cell);
      fragment.append(row);
    }

    elements.rows.replaceChildren(fragment);
    elements.empty.hidden = filtered.length !== 0;
    elements.resultCount.textContent = formatNumber(filtered.length) + " results";
    const rangeStart = filtered.length ? startIndex + 1 : 0;
    const rangeEnd = Math.min(startIndex + state.pageSize, filtered.length);
    elements.paginationSummary.textContent = "Showing " + formatNumber(rangeStart) +
      "–" + formatNumber(rangeEnd) + " of " + formatNumber(filtered.length) + " contributions";
    elements.pageIndicator.textContent = "Page " + state.page + " of " + pageCount;
    elements.previous.disabled = state.page <= 1;
    elements.next.disabled = state.page >= pageCount;
  }

  function showLoadError(message) {
    elements.rows.replaceChildren();
    const row = document.createElement("tr");
    const cell = createElement("td", "loading-cell", message);
    cell.colSpan = 4;
    row.append(cell);
    elements.rows.append(row);
    elements.resultCount.textContent = "Unavailable";
    elements.paginationSummary.textContent = "The contribution data could not be loaded.";
  }

  function setTheme(theme) {
    document.body.dataset.theme = theme;
    elements.themeToggle.textContent = theme === "light" ? "Dark mode" : "Light mode";
    elements.themeToggle.setAttribute("aria-label", "Switch to " + (theme === "light" ? "dark" : "light") + " theme");
    try {
      localStorage.setItem("oss-dashboard-theme", theme);
    } catch (error) {
      // The dashboard remains usable when browser storage is unavailable.
    }
  }

  function initTheme() {
    let savedTheme = "";
    try {
      savedTheme = localStorage.getItem("oss-dashboard-theme") || "";
    } catch (error) {
      savedTheme = "";
    }
    setTheme(savedTheme === "light" ? "light" : "dark");
    elements.themeToggle.addEventListener("click", () => {
      setTheme(document.body.dataset.theme === "light" ? "dark" : "light");
    });
  }

  function wireControls() {
    elements.search.addEventListener("input", () => {
      state.query = elements.search.value.trim();
      state.page = 1;
      renderRows();
    });
    elements.repository.addEventListener("change", () => {
      state.repository = elements.repository.value;
      state.page = 1;
      renderRows();
    });
    elements.sort.addEventListener("change", () => {
      state.sort = elements.sort.value;
      state.page = 1;
      renderRows();
    });
    elements.pageSize.addEventListener("change", () => {
      state.pageSize = Number(elements.pageSize.value) || 25;
      state.page = 1;
      renderRows();
    });
    elements.previous.addEventListener("click", () => {
      state.page = Math.max(1, state.page - 1);
      renderRows();
    });
    elements.next.addEventListener("click", () => {
      state.page += 1;
      renderRows();
    });
  }

  async function init() {
    initTheme();
    wireControls();

    try {
      const response = await fetch("data/contributions.json", { cache: "no-cache" });
      if (!response.ok) {
        throw new Error("The contribution dataset returned HTTP " + response.status + ".");
      }
      const ledger = await response.json();
      if (!ledger || !Array.isArray(ledger.contributions)) {
        throw new Error("The contribution dataset has an unexpected format.");
      }

      state.items = ledger.contributions.map(normaliseContribution).filter(Boolean);
      state.items.forEach((item) => {
        state.repositoryCounts.set(
          item.repository,
          (state.repositoryCounts.get(item.repository) || 0) + 1
        );
      });

      renderMetrics(ledger);
      populateRepositoryFilter();
      renderTopRepositories();
      renderRows();
    } catch (error) {
      showLoadError("Unable to load contributions. " + error.message);
    }
  }

  document.addEventListener("DOMContentLoaded", init);
})();
