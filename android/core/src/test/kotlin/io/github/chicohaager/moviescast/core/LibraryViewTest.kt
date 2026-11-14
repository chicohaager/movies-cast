package io.github.chicohaager.moviescast.core

import org.junit.Assert.assertEquals
import org.junit.Test

class LibraryViewTest {
    // In the server's order by name.
    private val library = listOf(
        MediaItem(id = "a", name = "Alpha", type = "Movie", year = 1999, score = 6.1, added = "2026-01-05T10:00:00Z", genres = listOf("Drama"), played = true),
        MediaItem(id = "b", name = "Beta", type = "Movie", year = 2020, score = 8.4, added = "2026-03-01T10:00:00Z", genres = listOf("Comedy", "Drama"), resumeSeconds = 90),
        MediaItem(id = "c", name = "Gamma", type = "Series", year = 2020, score = 7.0, added = "2025-12-24T10:00:00Z", genres = listOf("western"), favorite = true),
        MediaItem(id = "d", name = "Pilot", type = "Episode", seriesName = "Delta Show", year = 2011, added = "2026-03-01T10:00:00Z"),
    )

    private fun ids(sort: LibrarySort = LibrarySort.NAME, filter: LibraryFilter = LibraryFilter(), query: String = "") =
        library.arranged(query, sort, filter).map { it.id }

    @Test
    fun byNameIsTheServersOrder() = assertEquals(listOf("a", "b", "c", "d"), ids())

    @Test
    fun theOtherOrdersPutTheNewestLatestBestFirstAndKeepTheNameOrderForEqualValues() {
        assertEquals(listOf("b", "d", "a", "c"), ids(LibrarySort.ADDED))
        assertEquals(listOf("b", "c", "d", "a"), ids(LibrarySort.YEAR))
        assertEquals(listOf("b", "c", "a", "d"), ids(LibrarySort.SCORE))
    }

    @Test
    fun eachFilterKeepsItsItems() {
        assertEquals(listOf("b", "c", "d"), ids(filter = LibraryFilter(LibraryFilter.Kind.UNWATCHED)))
        assertEquals(listOf("b"), ids(filter = LibraryFilter(LibraryFilter.Kind.STARTED)))
        assertEquals(listOf("c"), ids(filter = LibraryFilter(LibraryFilter.Kind.FAVOURITES)))
        assertEquals(listOf("a", "b"), ids(filter = LibraryFilter(LibraryFilter.Kind.GENRE, "Drama")))
        assertEquals(listOf("b", "a"), ids(LibrarySort.YEAR, LibraryFilter(LibraryFilter.Kind.GENRE, "Drama")))
    }

    @Test
    fun theSearchLooksAtNameSeriesAndYear() {
        assertEquals(listOf("b"), ids(query = " beTA "))
        assertEquals(listOf("d"), ids(query = "delta"))
        assertEquals(listOf("b", "c"), ids(query = "2020"))
        assertEquals(listOf("b"), ids(query = "2020", filter = LibraryFilter(LibraryFilter.Kind.STARTED)))
    }

    @Test
    fun genresAreListedOnceInAlphabeticalOrder() = assertEquals(listOf("Comedy", "Drama", "western"), library.genres())

    @Test
    fun anEpisodeWithoutATitleOfItsOwnKeepsSeasonAndNumber() {
        assertEquals("S1 E2", episodeLine("S1 E2 · Test Show", "Test Show", "Test Show"))
        assertEquals("S1 E2", episodeLine("S1 E2 · ", "", "Test Show"))
        assertEquals("S1 E2 · The Pilot", episodeLine("S1 E2 · The Pilot", "The Pilot", "Test Show"))
    }

    @Test
    fun timesAreReadWithTheirOffset() {
        assertEquals(0L, instantSeconds("1970-01-01T00:00:00Z"))
        assertEquals(1772359200L, instantSeconds("2026-03-01T10:00:00.0000000Z"))
        // the same moment, written by a server in winter time and in summer time
        assertEquals(instantSeconds("2026-10-27T17:04:21Z"), instantSeconds("2026-10-27T18:04:21.329122815+01:00"))
        assertEquals(instantSeconds("2026-07-01T10:00:00Z"), instantSeconds("2026-07-01T12:00:00+02:00"))
        assertEquals(instantSeconds("2024-02-29T23:59:59Z") + 1, instantSeconds("2024-03-01T00:00:00Z"))
        assertEquals(0L, instantSeconds(""))
        assertEquals(0L, instantSeconds("yesterday"))
    }

    @Test
    fun recentlyAddedGoesByTheMomentNotByTheText() {
        // As texts the summer one is "later" (12:30 > 12:00); as moments it is earlier (10:30 UTC < 11:00 UTC).
        val two = listOf(
            MediaItem(id = "summer", name = "A", type = "Movie", added = "2026-07-01T12:30:00+02:00"),
            MediaItem(id = "winter", name = "B", type = "Movie", added = "2026-07-01T12:00:00+01:00"),
        )
        assertEquals(listOf("winter", "summer"), two.arranged("", LibrarySort.ADDED, LibraryFilter()).map { it.id })
    }

    @Test
    fun aLibraryTileShowsWhatIsInThatLibraryOnly() {
        val all = listOf(
            MediaItem(id = "a", name = "A", type = "Movie"),
            MediaItem(id = "b", name = "B", type = "Series"),
            MediaItem(id = "c", name = "C", type = "Movie"),
        )
        val library = MediaLibrary("v1", "Urlaub", "homevideos", setOf("c", "a"))
        val shown = all.arranged("", LibrarySort.NAME, LibraryFilter(LibraryFilter.Kind.LIBRARY, library = library))
        assertEquals(listOf("a", "c"), shown.map { it.id })
        // the tile of the whole library: everything
        assertEquals(listOf("a", "b", "c"), all.arranged("", LibrarySort.NAME, LibraryFilter(LibraryFilter.Kind.LIBRARY)).map { it.id })
    }
}
