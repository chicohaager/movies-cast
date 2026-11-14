package io.github.chicohaager.moviescast.core

/**
 * The rows of the home page above the whole library, as Infuse shows them (the owner's yardstick, ROADMAP A15) and as
 * Jellyfin's apps keep them: an ordered list the viewer sets, each row shown or not (Infuse "Home screen → Edit",
 * jellyfin-androidtv `homesection0…9`). A row with nothing in it is left out.
 */
enum class HomeRow { CONTINUE, LIBRARIES, FAVOURITES, RECENT, GENRES, TOP_RATED }

/** The rows in their order, each with whether it shows. */
data class HomeLayout(val rows: List<Pair<HomeRow, Boolean>> = DEFAULT) {
    val shown: List<HomeRow> get() = rows.filter { it.second }.map { it.first }

    fun toggle(row: HomeRow): HomeLayout = HomeLayout(rows.map { if (it.first == row) it.first to !it.second else it })

    fun moveUp(row: HomeRow): HomeLayout = move(row, -1)

    fun moveDown(row: HomeRow): HomeLayout = move(row, +1)

    private fun move(row: HomeRow, by: Int): HomeLayout {
        val at = rows.indexOfFirst { it.first == row }
        val to = at + by
        if (at < 0 || to !in rows.indices) return this
        val list = rows.toMutableList()
        list[at] = list[to].also { list[to] = list[at] }
        return HomeLayout(list)
    }

    /** "CONTINUE+,FAVOURITES-,…": the order, + shown, - hidden. */
    fun encode(): String = rows.joinToString(",") { "${it.first.name}${if (it.second) "+" else "-"}" }

    companion object {
        val DEFAULT: List<Pair<HomeRow, Boolean>> = HomeRow.entries.map { it to true }

        /**
         * What [encode] wrote. Unknown names (a row an older or newer build had) are dropped; a row the text does not
         * name (one added since) is put after the row it follows by default (else first) and shown. Empty or unreadable: the default.
         */
        fun decode(text: String?): HomeLayout {
            if (text.isNullOrBlank()) return HomeLayout()
            val read = text.split(",").mapNotNull { part ->
                val name = part.trim().dropLast(1)
                val row = HomeRow.entries.firstOrNull { it.name == name } ?: return@mapNotNull null
                when (part.trim().lastOrNull()) {
                    '+' -> row to true
                    '-' -> row to false
                    else -> null
                }
            }.distinctBy { it.first }
            val list = read.toMutableList()
            for ((index, row) in HomeRow.entries.withIndex()) {
                if (list.any { it.first == row }) continue
                // after the row that comes before it by default, else first
                val before = if (index == 0) -1 else list.indexOfFirst { it.first == HomeRow.entries[index - 1] }
                list.add(before + 1, row to true)
            }
            return HomeLayout(list)
        }
    }
}

/** How many items a row holds at most. */
const val HOME_ROW_SIZE = 20

/** The favourites, in the library's order. */
fun List<MediaItem>.favourites(): List<MediaItem> = filter { it.favorite }.take(HOME_ROW_SIZE)

/** What the server took in last, newest first; items without a date are left out. */
fun List<MediaItem>.recentlyAdded(): List<MediaItem> =
    filter { instantSeconds(it.added) > 0 }.sortedByDescending { instantSeconds(it.added) }.take(HOME_ROW_SIZE)

/** The best rated (the server's `CommunityRating`), best first; items without a rating are left out. */
fun List<MediaItem>.topRated(): List<MediaItem> = filter { it.score > 0 }.sortedByDescending { it.score }.take(HOME_ROW_SIZE)

/** A genre as a picture tile: its name and the item whose picture it shows (null: no item of it has one). */
data class GenreTile(val genre: String, val picture: MediaItem?)

/**
 * Every genre of the library as a tile, alphabetically. The picture is the backdrop of the best rated item of the
 * genre that has one, else a poster; an item is used for one tile only while another is at hand, so the tiles do not
 * all show the same film.
 */
fun List<MediaItem>.genreTiles(): List<GenreTile> {
    val used = HashSet<String>()
    return genres().map { genre ->
        val members = filter { genre in it.genres }.sortedByDescending { it.score }
        val withBackdrop = members.filter { it.backdropItemId.isNotEmpty() }
        val pick = withBackdrop.firstOrNull { it.id !in used } ?: withBackdrop.firstOrNull()
            ?: members.filter { it.hasPoster }.let { posters -> posters.firstOrNull { it.id !in used } ?: posters.firstOrNull() }
        pick?.let { used.add(it.id) }
        GenreTile(genre, pick)
    }
}

/**
 * The tiles of "My media", as Infuse's row of shortcut tiles (Bibliothek, Filme, TV-Serien, Andere) and Jellyfin's
 * "My media": the server's libraries in its order that hold something the app lists; a library with nothing of
 * [items] in it is left out (Movies keeps an empty "Serien"). The tile of the whole library comes before them in the UI.
 */
fun List<MediaItem>.libraryTiles(libraries: List<MediaLibrary>): List<MediaLibrary> {
    val ids = mapTo(HashSet()) { it.id }
    return libraries.filter { library -> library.members.any { it in ids } }
}
