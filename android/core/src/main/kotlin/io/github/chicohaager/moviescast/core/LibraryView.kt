package io.github.chicohaager.moviescast.core

/** The order of the library, as on movies-cast's phone page: by name, or the newest, latest or best first. */
enum class LibrarySort { NAME, ADDED, YEAR, SCORE }

/** Which items of the library show: all, or one of the kinds below; [genre] counts with [Kind.GENRE] only. */
data class LibraryFilter(val kind: Kind = Kind.ALL, val genre: String = "") {
    enum class Kind { ALL, UNWATCHED, STARTED, FAVOURITES, GENRE }
}

/**
 * The library as the screen shows it. The server is not asked again: Movies ignores `IsPlayed` and
 * `Genres` as filters (measured 2026-10-31 for movies-cast's web page, which filters the same way),
 * and the whole library is in memory anyway.
 *
 * The search looks at the name, the series' name and the year. By name is the server's own order
 * (its `SortName`); the other orders fall back to it for equal values.
 */
fun List<MediaItem>.arranged(query: String, sort: LibrarySort, filter: LibraryFilter): List<MediaItem> {
    val q = query.trim().lowercase()
    val kept = filter { item ->
        (q.isEmpty() || "${item.name} ${item.seriesName} ${if (item.year > 0) item.year else ""}".lowercase().contains(q)) &&
            when (filter.kind) {
                LibraryFilter.Kind.ALL -> true
                LibraryFilter.Kind.UNWATCHED -> !item.played
                LibraryFilter.Kind.STARTED -> item.resumeSeconds > 0
                LibraryFilter.Kind.FAVOURITES -> item.favorite
                LibraryFilter.Kind.GENRE -> filter.genre in item.genres
            }
    }
    // sortedBy... is stable, so equal values keep the server's order by name.
    return when (sort) {
        LibrarySort.NAME -> kept
        LibrarySort.ADDED -> kept.sortedByDescending { instantSeconds(it.added) }
        LibrarySort.YEAR -> kept.sortedByDescending { it.year }
        LibrarySort.SCORE -> kept.sortedByDescending { it.score }
    }
}

private val isoTime = Regex("""(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.\d+)?(Z|[+-]\d{2}:\d{2})?""")

/**
 * Seconds since 1970 (UTC) of a time as the servers write it: Jellyfin `2026-03-01T10:00:00.0000000Z`,
 * Movies `2026-10-27T18:04:21.329122815+01:00` (measured 2026-11-13). The texts cannot be compared
 * as they are: Movies writes local time with its offset, which differs between summer and winter.
 * Done by hand because `java.time` needs Android 8 and the app runs from Android 6. 0 for anything else.
 */
fun instantSeconds(text: String): Long {
    val m = isoTime.matchEntire(text.trim()) ?: return 0
    val (year, month, day) = m.groupValues.subList(1, 4).map { it.toLong() }
    val (hour, minute, second) = m.groupValues.subList(4, 7).map { it.toLong() }
    // days from the civil date (Howard Hinnant's algorithm)
    val y = if (month <= 2) year - 1 else year
    val era = (if (y >= 0) y else y - 399) / 400
    val yoe = y - era * 400
    val doy = (153 * (month + (if (month > 2) -3 else 9)) + 2) / 5 + day - 1
    val doe = yoe * 365 + yoe / 4 - yoe / 100 + doy
    val days = era * 146097 + doe - 719468
    val zone = m.groupValues[7]
    val offset = if (zone.isEmpty() || zone == "Z") 0 else (if (zone[0] == '-') -1 else 1) * (zone.substring(1, 3).toLong() * 3600 + zone.substring(4, 6).toLong() * 60)
    return days * 86400 + hour * 3600 + minute * 60 + second - offset
}

/** Every genre of the library, once, in alphabetical order. */
fun List<MediaItem>.genres(): List<String> = flatMap { it.genres }.distinct().sortedWith(String.CASE_INSENSITIVE_ORDER)

/**
 * The line an episode is listed with. [label] is the formatted text "S1 E2 · name"; an episode
 * without a title of its own (Movies names it like its series, measured 2026-11-13) keeps "S1 E2".
 */
fun episodeLine(label: String, name: String, seriesName: String): String =
    if (name.isBlank() || name == seriesName) label.removeSuffix(" · $name").trimEnd() else label
