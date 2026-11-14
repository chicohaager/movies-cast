package io.github.chicohaager.moviescast.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** The home page's rows: what each holds, and the order and choice the viewer sets. Invented items only. */
class HomeRowsTest {
    private fun item(
        id: String,
        added: String = "",
        score: Double = 0.0,
        favorite: Boolean = false,
        genres: List<String> = emptyList(),
        backdrop: Boolean = false,
        poster: Boolean = false,
    ) = MediaItem(
        id = id, name = id.uppercase(), type = "Movie", added = added, score = score, favorite = favorite, genres = genres,
        backdropItemId = if (backdrop) id else "", backdropTag = if (backdrop) "t-$id" else "", hasPoster = poster,
    )

    @Test
    fun recentlyAddedIsNewestFirstByTheMomentNotTheText() {
        // Movies writes local time with its offset (measured 2026-11-13): b is the later moment though its text sorts first
        val library = listOf(
            item("a", added = "2026-10-27T18:04:21.329122815+01:00"),
            item("b", added = "2026-10-27T18:30:00+02:00"),
            item("c", added = "2026-11-01T09:00:00Z"),
            item("d"),
        )
        assertEquals(listOf("c", "a", "b"), library.recentlyAdded().map { it.id })
    }

    @Test
    fun topRatedIsBestFirstWithoutTheUnrated() {
        val library = listOf(item("a", score = 6.1), item("b"), item("c", score = 7.4), item("d", score = 6.9))
        assertEquals(listOf("c", "d", "a"), library.topRated().map { it.id })
    }

    @Test
    fun favouritesKeepTheLibrarysOrder() {
        val library = listOf(item("a", favorite = true), item("b"), item("c", favorite = true))
        assertEquals(listOf("a", "c"), library.favourites().map { it.id })
    }

    @Test
    fun aRowHoldsAtMostTwenty() {
        val library = (1..30).map { item("i$it", score = it.toDouble(), favorite = true, added = "2026-11-%02dT10:00:00Z".format(it)) }
        assertEquals(HOME_ROW_SIZE, library.topRated().size)
        assertEquals(HOME_ROW_SIZE, library.favourites().size)
        assertEquals(HOME_ROW_SIZE, library.recentlyAdded().size)
        assertEquals("i30", library.recentlyAdded().first().id)
    }

    @Test
    fun aGenreTileShowsTheBestRatedBackdropAndNotTheSameFilmTwiceWhileAnotherIsThere() {
        val library = listOf(
            item("rocky", score = 7.4, genres = listOf("Comedy", "Horror"), backdrop = true),
            item("shop", score = 6.9, genres = listOf("Comedy", "Horror"), backdrop = true),
            item("holiday", score = 9.0, genres = listOf("Travel"), poster = true),
            item("bare", genres = listOf("Music")),
        )
        val tiles = library.genreTiles()
        assertEquals(listOf("Comedy", "Horror", "Music", "Travel"), tiles.map { it.genre })
        assertEquals("rocky", tiles[0].picture?.id)
        assertEquals("shop", tiles[1].picture?.id)
        assertNull(tiles[2].picture)
        assertEquals("holiday", tiles[3].picture?.id) // no backdrop in the genre: its poster
    }

    @Test
    fun theLayoutMovesAndHidesRowsAndComesBackFromItsText() {
        var layout = HomeLayout()
        assertEquals(HomeRow.entries.toList(), layout.shown)
        layout = layout.moveDown(HomeRow.CONTINUE).toggle(HomeRow.GENRES).moveUp(HomeRow.TOP_RATED)
        assertEquals(listOf(HomeRow.FAVOURITES, HomeRow.CONTINUE, HomeRow.RECENT, HomeRow.TOP_RATED, HomeRow.GENRES), layout.rows.map { it.first })
        assertEquals(listOf(HomeRow.FAVOURITES, HomeRow.CONTINUE, HomeRow.RECENT, HomeRow.TOP_RATED), layout.shown)
        assertEquals(layout, HomeLayout.decode(layout.encode()))
        // at the ends nothing moves
        assertEquals(layout, layout.moveUp(HomeRow.FAVOURITES))
        assertEquals(layout, layout.moveDown(HomeRow.GENRES))
    }

    @Test
    fun aStoredLayoutFromAnotherBuildIsReadSafely() {
        assertEquals(HomeLayout(), HomeLayout.decode(null))
        assertEquals(HomeLayout(), HomeLayout.decode(""))
        assertEquals(HomeLayout(), HomeLayout.decode("garbage"))
        // a row this build does not know is dropped; rows the text does not name come back at their place, shown
        val read = HomeLayout.decode("TOP_RATED-,SPOTLIGHT+,CONTINUE+")
        assertEquals(listOf(HomeRow.TOP_RATED, HomeRow.CONTINUE, HomeRow.FAVOURITES, HomeRow.RECENT, HomeRow.GENRES), read.rows.map { it.first })
        assertEquals(listOf(HomeRow.CONTINUE, HomeRow.FAVOURITES, HomeRow.RECENT, HomeRow.GENRES), read.shown)
        // a row named twice counts once
        assertEquals(1, HomeLayout.decode("GENRES-,GENRES+").rows.count { it.first == HomeRow.GENRES })
    }
}
