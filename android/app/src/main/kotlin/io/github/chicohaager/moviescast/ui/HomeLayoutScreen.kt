package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.core.HomeLayout
import io.github.chicohaager.moviescast.core.HomeRow

/** The name of a home row, as its heading on the home page. */
@Composable
fun homeRowName(row: HomeRow): String = stringResource(
    when (row) {
        HomeRow.CONTINUE -> R.string.library_continue
        HomeRow.LIBRARIES -> R.string.home_libraries
        HomeRow.FAVOURITES -> R.string.filter_favourites
        HomeRow.RECENT -> R.string.sort_added
        HomeRow.GENRES -> R.string.home_genres
        HomeRow.TOP_RATED -> R.string.home_top_rated
    },
)

/**
 * "Home screen" in the menu: which rows stand above the library and in what order, as Infuse's "Home screen → Edit".
 * Every control is a button, so a remote reaches each one and OK works on it; a row's switch turns with OK on the row.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeLayoutScreen(layout: HomeLayout, onBack: () -> Unit, onChange: (HomeLayout) -> Unit) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.home_screen)) },
                navigationIcon = { TextButton(onClick = onBack, modifier = Modifier.testTag("back")) { Text("‹ " + stringResource(R.string.back)) } },
            )
        },
    ) { padding ->
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 24.dp, vertical = 12.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            val up = stringResource(R.string.home_move_up)
            val down = stringResource(R.string.home_move_down)
            layout.rows.forEachIndexed { index, (row, shown) ->
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    SoftButton(
                        onClick = { onChange(layout.toggle(row)) },
                        shape = RoundedCornerShape(12.dp),
                        modifier = Modifier.weight(1f).testTag("home_row_${row.name}"),
                    ) {
                        Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                            Text(homeRowName(row), style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
                            Switch(checked = shown, onCheckedChange = null)
                        }
                    }
                    SoftButton(
                        onClick = { onChange(layout.moveUp(row)) },
                        enabled = index > 0,
                        modifier = Modifier.semantics { contentDescription = up }.testTag("home_up_${row.name}"),
                    ) { Text("↑") }
                    SoftButton(
                        onClick = { onChange(layout.moveDown(row)) },
                        enabled = index < layout.rows.lastIndex,
                        modifier = Modifier.semantics { contentDescription = down }.testTag("home_down_${row.name}"),
                    ) { Text("↓") }
                }
            }
            SoftButton(onClick = { onChange(HomeLayout()) }, modifier = Modifier.padding(top = 8.dp).testTag("home_reset")) {
                Text(stringResource(R.string.home_reset))
            }
        }
    }
}
