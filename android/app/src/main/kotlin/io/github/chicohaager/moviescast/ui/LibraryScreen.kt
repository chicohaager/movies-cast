package io.github.chicohaager.moviescast.ui

import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.withFrameNanos
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiError
import io.github.chicohaager.moviescast.UiState
import io.github.chicohaager.moviescast.core.LibraryFilter
import io.github.chicohaager.moviescast.core.LibrarySort
import io.github.chicohaager.moviescast.core.MediaItem
import io.github.chicohaager.moviescast.core.genres
import io.github.chicohaager.moviescast.core.GenreTile
import io.github.chicohaager.moviescast.core.HomeRow
import io.github.chicohaager.moviescast.core.favourites
import io.github.chicohaager.moviescast.core.genreTiles
import io.github.chicohaager.moviescast.core.recentlyAdded
import io.github.chicohaager.moviescast.core.topRated
import io.github.chicohaager.moviescast.MoviesCastApp
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.lazy.grid.LazyGridScope
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import coil3.compose.AsyncImage

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun LibraryScreen(
    state: UiState,
    onSearch: (String) -> Unit,
    onSort: (LibrarySort) -> Unit,
    onFilter: (LibraryFilter) -> Unit,
    onOpen: (MediaItem) -> Unit,
    onRefresh: () -> Unit,
    onLogout: () -> Unit,
    onOpenSettings: () -> Unit,
    onAccounts: () -> Unit,
    onSound: () -> Unit = {},
    onHome: () -> Unit = {},
    onAbout: () -> Unit,
) {
    val searchField = rememberRemoteField()
    val grid = rememberLazyGridState()
    // OK on a genre tile filters the library and the tile is gone: the focus goes to the first film found, not to
    // the top bar where the focus search put it (Android TV emulator, 2026-11-14).
    val firstFound = remember { FocusRequester() }
    var focusFound by remember { mutableStateOf(false) }
    LaunchedEffect(focusFound, state.shown) {
        if (focusFound && state.shown.isNotEmpty()) {
            grid.scrollToItem(0)
            withFrameNanos { }
            runCatching { firstFound.requestFocus() }
            focusFound = false
        }
    }
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(14.dp)) {
                        Wordmark(stringResource(R.string.app_name), markSize = 34.dp, textSize = 17)
                        if (state.userName.isNotEmpty()) {
                            Text(
                                state.userName,
                                style = MaterialTheme.typography.labelMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                                modifier = Modifier
                                    .border(1.dp, MaterialTheme.colorScheme.outlineVariant, RoundedCornerShape(50))
                                    .padding(horizontal = 10.dp, vertical = 3.dp),
                            )
                        }
                    }
                },
                actions = {
                    TextButton(onClick = onRefresh, modifier = Modifier.testTag("reload")) { Text(stringResource(R.string.library_refresh)) }
                    // Three words do not fit beside the title in every language (French, measured 2026-11-12).
                    var menu by rememberSaveable { mutableStateOf(false) }
                    var languages by rememberSaveable { mutableStateOf(false) }
                    val menuName = stringResource(R.string.menu)
                    TextButton(
                        onClick = { menu = true },
                        modifier = Modifier.testTag("menu").semantics { contentDescription = menuName },
                    ) { Text("⋮", style = MaterialTheme.typography.titleLarge) }
                    DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.language)) },
                            onClick = { menu = false; languages = true },
                            modifier = Modifier.testTag("language"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.accounts)) },
                            onClick = { menu = false; onAccounts() },
                            modifier = Modifier.testTag("accounts"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.home_screen)) },
                            onClick = { menu = false; onHome() },
                            modifier = Modifier.testTag("home"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.sound)) },
                            onClick = { menu = false; onSound() },
                            modifier = Modifier.testTag("sound"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.about)) },
                            onClick = { menu = false; onAbout() },
                            modifier = Modifier.testTag("about"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.logout)) },
                            onClick = { menu = false; onLogout() },
                            modifier = Modifier.testTag("logout"),
                        )
                    }
                    if (languages) LanguageDialog { languages = false }
                },
            )
        },
    ) { padding ->
        PullToRefreshBox(isRefreshing = state.busy, onRefresh = onRefresh, modifier = Modifier.padding(padding).fillMaxSize()) {
            LazyVerticalGrid(
                columns = GridCells.Adaptive(minSize = 116.dp),
                contentPadding = PaddingValues(start = 16.dp, end = 16.dp, bottom = 24.dp),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                verticalArrangement = Arrangement.spacedBy(16.dp),
                state = grid,
                modifier = Modifier.fillMaxSize().testTag("library"),
            ) {
                item(span = { GridItemSpan(maxLineSpan) }) {
                    OutlinedTextField(
                        value = state.query,
                        onValueChange = onSearch,
                        label = { Text(stringResource(R.string.library_search)) },
                        singleLine = true,
                        shape = RoundedCornerShape(50),
                        colors = OutlinedTextFieldDefaults.colors(
                            unfocusedContainerColor = MaterialTheme.colorScheme.surfaceContainerLow.copy(alpha = 0.7f),
                            focusedContainerColor = MaterialTheme.colorScheme.surfaceContainer.copy(alpha = 0.85f),
                            unfocusedBorderColor = MaterialTheme.colorScheme.outlineVariant,
                            focusedBorderColor = MaterialTheme.colorScheme.secondary,
                        ),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                        readOnly = searchField.readOnly,
                        modifier = Modifier.fillMaxWidth().then(searchField.modifier).testTag("search"),
                    )
                }
                item(span = { GridItemSpan(maxLineSpan) }) { SortAndFilter(state, onSort, onFilter) }
                state.error?.let { error ->
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                            Text(errorText(error), color = MaterialTheme.colorScheme.error)
                            Button(onClick = onRefresh) { Text(stringResource(R.string.retry)) }
                            if (error == UiError.LocalNetwork) {
                                SoftButton(onClick = onOpenSettings) { Text(stringResource(R.string.open_settings)) }
                            }
                        }
                    }
                }
                // The rows above the library, as the person set them (menu "Home screen"); only on the plain list,
                // a search or a filter shows its result alone. A row with nothing in it is left out.
                if (state.plain) {
                    for (row in state.home.shown) {
                        when (row) {
                            HomeRow.CONTINUE -> posterRow(row, state.resume, onOpen, showProgress = true)
                            HomeRow.FAVOURITES -> posterRow(row, state.library.favourites(), onOpen)
                            HomeRow.RECENT -> posterRow(row, state.library.recentlyAdded(), onOpen)
                            HomeRow.TOP_RATED -> posterRow(row, state.library.topRated(), onOpen)
                            HomeRow.GENRES -> {
                                val tiles = state.library.genreTiles()
                                if (tiles.isNotEmpty()) {
                                    item(span = { GridItemSpan(maxLineSpan) }) { Heading(homeRowName(row), Modifier.testTag("row_${row.name}")) }
                                    item(span = { GridItemSpan(maxLineSpan) }) {
                                        LazyRow(horizontalArrangement = Arrangement.spacedBy(14.dp), contentPadding = PaddingValues(vertical = 8.dp)) {
                                            items(tiles, key = { it.genre }) { tile ->
                                                GenreCard(tile) {
                                                    focusFound = true
                                                    onFilter(LibraryFilter(LibraryFilter.Kind.GENRE, tile.genre))
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
                if (state.loaded) {
                    item(span = { GridItemSpan(maxLineSpan) }) { Heading(stringResource(R.string.library_all)) }
                    if (state.shown.isEmpty()) {
                        item(span = { GridItemSpan(maxLineSpan) }) {
                            Text(
                                if (state.library.isEmpty()) stringResource(R.string.library_empty)
                                else if (state.query.isBlank()) stringResource(R.string.library_none)
                                else stringResource(R.string.library_no_match, state.query.trim()),
                                color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
                itemsIndexed(state.shown, key = { _, it -> it.id }) { index, item ->
                    Tile(item, if (index == 0) Modifier.focusRequester(firstFound) else Modifier, onOpen, showProgress = false)
                }
            }
        }
    }
}

/** Two menus as on movies-cast's phone page: the order, and which items show (the genres come from the library). */
@Composable
private fun SortAndFilter(state: UiState, onSort: (LibrarySort) -> Unit, onFilter: (LibraryFilter) -> Unit) {
    val sorts = listOf(
        LibrarySort.NAME to R.string.sort_name,
        LibrarySort.ADDED to R.string.sort_added,
        LibrarySort.YEAR to R.string.sort_year,
        LibrarySort.SCORE to R.string.sort_score,
    )
    val kinds = listOf(
        LibraryFilter.Kind.ALL to R.string.filter_all,
        LibraryFilter.Kind.UNWATCHED to R.string.filter_unwatched,
        LibraryFilter.Kind.STARTED to R.string.filter_started,
        LibraryFilter.Kind.FAVOURITES to R.string.filter_favourites,
    )
    val genres = remember(state.library) { state.library.genres() }
    val filterName = if (state.filter.kind == LibraryFilter.Kind.GENRE) stringResource(R.string.filter_genre, state.filter.genre)
    else stringResource(kinds.first { it.first == state.filter.kind }.second)
    Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        Choice("sort", stringResource(R.string.sort_label), stringResource(sorts.first { it.first == state.sort }.second)) { close ->
            for ((sort, name) in sorts) {
                DropdownMenuItem(text = { Text(stringResource(name)) }, onClick = { close(); onSort(sort) }, modifier = Modifier.testTag("sort_${sort.name}"))
            }
        }
        Choice("filter", stringResource(R.string.filter_label), filterName) { close ->
            for ((kind, name) in kinds) {
                DropdownMenuItem(text = { Text(stringResource(name)) }, onClick = { close(); onFilter(LibraryFilter(kind)) }, modifier = Modifier.testTag("filter_${kind.name}"))
            }
            for (genre in genres) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.filter_genre, genre)) },
                    onClick = { close(); onFilter(LibraryFilter(LibraryFilter.Kind.GENRE, genre)) },
                    modifier = Modifier.testTag("filter_genre"),
                )
            }
        }
    }
}

/** A button "label: value" that opens a menu; [items] gets a function that closes it. */
@Composable
private fun Choice(tag: String, label: String, value: String, items: @Composable (close: () -> Unit) -> Unit) {
    var open by rememberSaveable { mutableStateOf(false) }
    Box {
        TextButton(onClick = { open = true }, modifier = Modifier.testTag(tag)) { Text("$label: $value ▾", maxLines = 1, overflow = TextOverflow.Ellipsis) }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) { items { open = false } }
    }
}

/** A row of posters under its heading; nothing when [items] is empty. */
private fun LazyGridScope.posterRow(row: HomeRow, items: List<MediaItem>, onOpen: (MediaItem) -> Unit, showProgress: Boolean = false) {
    if (items.isEmpty()) return
    item(span = { GridItemSpan(maxLineSpan) }) { Heading(homeRowName(row), Modifier.testTag("row_${row.name}")) }
    item(span = { GridItemSpan(maxLineSpan) }) {
        LazyRow(horizontalArrangement = Arrangement.spacedBy(12.dp), contentPadding = PaddingValues(vertical = 8.dp)) {
            items(items, key = { it.id }) { item -> Tile(item, Modifier.width(132.dp), onOpen, showProgress = showProgress) }
        }
    }
}

/**
 * A genre as a picture tile, as Infuse's "Film genres": the backdrop (else the poster) of a film of the genre under a
 * veil, the genre's name over it. OK on it shows the library filtered by the genre.
 */
@Composable
private fun GenreCard(tile: GenreTile, onClick: () -> Unit) {
    val app = LocalContext.current.applicationContext as MoviesCastApp
    val session = app.session
    val picture = tile.picture
    val url = if (session == null || picture == null) null
    else app.client.backdropUrl(session, picture, 640) ?: app.client.posterUrl(session, picture.id, 400)
    var focused by remember { mutableStateOf(false) }
    val scale by animateFloatAsState(if (focused) 1.06f else 1f, label = "genre")
    Box(
        Modifier.width(240.dp).aspectRatio(16f / 9f)
            .onFocusChanged { focused = it.isFocused }
            .graphicsLayer { scaleX = scale; scaleY = scale }
            .border(3.dp, if (focused) Brand.Planet else SolidColor(Color.Transparent), RoundedCornerShape(16.dp))
            .padding(3.dp)
            .clip(RoundedCornerShape(13.dp))
            .background(Brush.linearGradient(listOf(MaterialTheme.colorScheme.surfaceContainerHigh, MaterialTheme.colorScheme.surfaceVariant)))
            .clickable(onClick = onClick)
            .testTag("genre"),
        contentAlignment = Alignment.Center,
    ) {
        if (url != null) {
            AsyncImage(model = url, contentDescription = null, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize())
            Box(Modifier.fillMaxSize().background(Brand.Night.copy(alpha = 0.45f)))
        }
        Text(
            tile.genre,
            color = Color.White,
            style = MaterialTheme.typography.titleLarge,
            fontWeight = FontWeight.Bold,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.padding(8.dp),
        )
    }
}

/** A section title with a short bar in the planet's orange before it. */
@Composable
private fun Heading(text: String, modifier: Modifier = Modifier) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = modifier.padding(top = 12.dp)) {
        Box(Modifier.width(4.dp).height(22.dp).background(Brand.Planet, RoundedCornerShape(50)))
        Text(text, style = MaterialTheme.typography.titleLarge, modifier = Modifier.padding(start = 10.dp))
    }
}

@Composable
private fun Tile(item: MediaItem, modifier: Modifier, onOpen: (MediaItem) -> Unit, showProgress: Boolean) {
    var focused by remember { mutableStateOf(false) }
    // A remote control has no pointer: the tile in focus grows a little and gets a glowing frame in the planet's orange.
    val scale by animateFloatAsState(if (focused) 1.06f else 1f, label = "tile")
    Column(
        modifier
            .onFocusChanged { focused = it.isFocused }
            .graphicsLayer { scaleX = scale; scaleY = scale }
            .border(3.dp, if (focused) Brand.Planet else SolidColor(Color.Transparent), RoundedCornerShape(16.dp))
            .padding(3.dp)
            .clip(RoundedCornerShape(13.dp))
            .clickable { onOpen(item) }
            .testTag("tile"),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        Box(contentAlignment = Alignment.BottomCenter) {
            Poster(item, Modifier.fillMaxWidth())
            if (item.played) {
                val watched = stringResource(R.string.watched)
                Text(
                    "✓",
                    color = Color.White,
                    style = MaterialTheme.typography.labelLarge,
                    modifier = Modifier.align(Alignment.TopEnd).padding(6.dp)
                        .background(Brand.Planet, RoundedCornerShape(50))
                        .padding(horizontal = 7.dp, vertical = 2.dp)
                        .semantics { contentDescription = watched }
                        .testTag("watched"),
                )
            }
            if (showProgress && item.runtimeSeconds > 0) {
                LinearProgressIndicator(
                    progress = { (item.resumeSeconds.toFloat() / item.runtimeSeconds).coerceIn(0f, 1f) },
                    color = Brand.Sky,
                    trackColor = Color.White.copy(alpha = 0.18f),
                    drawStopIndicator = {},
                    gapSize = 0.dp,
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp).height(4.dp).clip(RoundedCornerShape(50)),
                )
            }
        }
        Text(item.name, style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium, maxLines = 2, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(top = 4.dp))
        if (item.year > 0) {
            Text(item.year.toString(), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}
