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
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiState

/**
 * The logins the device keeps: every server and account the person logged in to. A tap puts one
 * in use without typing its password again; "Remove" logs it out at its server and forgets it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountsScreen(
    state: UiState,
    /** Null when this is the first page (no login in use). */
    onBack: (() -> Unit)?,
    onChoose: (Int) -> Unit,
    onRemove: (Int) -> Unit,
    onAdd: () -> Unit,
) {
    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text(stringResource(R.string.accounts), maxLines = 1, overflow = TextOverflow.Ellipsis) },
                navigationIcon = {
                    if (onBack != null) TextButton(onClick = onBack, modifier = Modifier.testTag("back")) { Text("‹ " + stringResource(R.string.back)) }
                },
            )
        },
    ) { padding ->
        Column(
            Modifier.padding(padding).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            state.error?.let {
                Text(errorText(it), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
            }
            state.logins.forEachIndexed { index, login ->
                Row(verticalAlignment = Alignment.CenterVertically) {
                    SoftButton(
                        onClick = { onChoose(index) },
                        // two lines, the first may wrap: the round ends of a button would cut into them
                        shape = RoundedCornerShape(12.dp),
                        modifier = Modifier.weight(1f).testTag("account"),
                    ) {
                        Column(Modifier.fillMaxWidth()) {
                            Text((if (index == state.current) "✓  " else "") + login.title, style = MaterialTheme.typography.bodyLarge)
                            Text(login.baseUrl, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                    TextButton(onClick = { onRemove(index) }, modifier = Modifier.testTag("account_remove")) {
                        Text(stringResource(R.string.account_remove))
                    }
                }
            }
            Button(onClick = onAdd, modifier = Modifier.fillMaxWidth().testTag("account_add")) {
                Text(stringResource(R.string.account_add))
            }
        }
    }
}
