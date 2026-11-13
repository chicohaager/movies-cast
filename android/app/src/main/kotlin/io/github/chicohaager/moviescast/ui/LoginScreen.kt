package io.github.chicohaager.moviescast.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import io.github.chicohaager.moviescast.R
import io.github.chicohaager.moviescast.UiError
import io.github.chicohaager.moviescast.UiState
import io.github.chicohaager.moviescast.core.FoundServer

@Composable
fun LoginScreen(
    state: UiState,
    lastServer: String,
    lastUser: String,
    onLogin: (String, String, String) -> Unit,
    onFind: () -> Unit,
    onOpenSettings: () -> Unit,
    /** Back to the list of the logins kept; null when there is none to go back to. */
    onAccounts: (() -> Unit)?,
    onAbout: () -> Unit,
) {
    var server by rememberSaveable { mutableStateOf(lastServer) }
    var user by rememberSaveable { mutableStateOf(lastUser) }
    // Not saved across process death on purpose: a password does not belong in the saved state.
    var password by androidx.compose.runtime.remember { mutableStateOf("") }
    val ready = server.isNotBlank() && user.isNotBlank() && !state.busy
    val submit = { if (ready) onLogin(server, user, password) }
    val serverField = rememberRemoteField()
    val userField = rememberRemoteField()
    val passwordField = rememberRemoteField()

    // On the sky of [SpaceBackground]; the form sits on a frosted panel.
    Box(Modifier.fillMaxSize().safeDrawingPadding().imePadding(), contentAlignment = Alignment.Center) {
        Column(
            Modifier.widthIn(max = 460.dp).fillMaxWidth().verticalScroll(rememberScrollState()).padding(20.dp)
                .background(MaterialTheme.colorScheme.surfaceContainerLow.copy(alpha = 0.72f), RoundedCornerShape(28.dp))
                .border(1.dp, MaterialTheme.colorScheme.outlineVariant, RoundedCornerShape(28.dp))
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp),
        ) {
            onAccounts?.let {
                TextButton(onClick = it, modifier = Modifier.testTag("accounts")) { Text("‹ " + stringResource(R.string.accounts)) }
            }
            Wordmark(stringResource(R.string.app_name), markSize = 64.dp, textSize = 30)
            Text(stringResource(R.string.login_title), style = MaterialTheme.typography.headlineMedium)
            Text(
                stringResource(R.string.login_hint),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            OutlinedTextField(
                value = server,
                onValueChange = { server = it },
                label = { Text(stringResource(R.string.login_server)) },
                placeholder = { Text(stringResource(R.string.login_server_example)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(
                    capitalization = KeyboardCapitalization.None,
                    autoCorrectEnabled = false,
                    keyboardType = KeyboardType.Uri,
                    imeAction = ImeAction.Next,
                ),
                readOnly = serverField.readOnly,
                modifier = Modifier.fillMaxWidth().then(serverField.modifier).testTag("server"),
            )
            OutlinedButton(
                onClick = onFind,
                enabled = !state.searching && !state.busy,
                modifier = Modifier.fillMaxWidth().testTag("find"),
            ) {
                if (state.searching) {
                    CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                } else {
                    Text(stringResource(R.string.find_servers))
                }
            }
            state.found?.let { found ->
                if (found.isEmpty()) {
                    Text(
                        stringResource(R.string.find_none),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.testTag("find_none"),
                    )
                }
                // A tap puts the address into the field above; account and password stay to be typed.
                found.forEach { it ->
                    OutlinedButton(
                        onClick = { server = it.address },
                        // two lines, the first may wrap: the round ends of a button would cut into them
                        shape = RoundedCornerShape(12.dp),
                        modifier = Modifier.fillMaxWidth().testTag("found"),
                    ) {
                        Column(Modifier.fillMaxWidth()) {
                            Text(foundTitle(it), style = MaterialTheme.typography.bodyLarge)
                            Text(it.address, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
            OutlinedTextField(
                value = user,
                onValueChange = { user = it },
                label = { Text(stringResource(R.string.login_user)) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(
                    capitalization = KeyboardCapitalization.None,
                    autoCorrectEnabled = false,
                    imeAction = ImeAction.Next,
                ),
                readOnly = userField.readOnly,
                modifier = Modifier.fillMaxWidth().then(userField.modifier).testTag("user"),
            )
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text(stringResource(R.string.login_password)) },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Go),
                keyboardActions = KeyboardActions(onGo = { submit() }),
                readOnly = passwordField.readOnly,
                modifier = Modifier.fillMaxWidth().then(passwordField.modifier).testTag("password"),
            )
            state.error?.let {
                Text(errorText(it), color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium)
                if (it == UiError.LocalNetwork) {
                    OutlinedButton(onClick = onOpenSettings, modifier = Modifier.testTag("settings")) {
                        Text(stringResource(R.string.open_settings))
                    }
                }
            }
            GlowButton(onClick = submit, enabled = ready, modifier = Modifier.fillMaxWidth().height(52.dp).testTag("login")) {
                if (state.busy) {
                    CircularProgressIndicator(Modifier.size(22.dp), strokeWidth = 2.dp)
                } else {
                    Text(stringResource(R.string.login_button))
                }
            }
            // Who cannot log in needs "Share diagnostics" most, so About is reachable from here too.
            Row(Modifier.align(Alignment.CenterHorizontally), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                LanguageButton()
                TextButton(onClick = onAbout, modifier = Modifier.testTag("about")) { Text(stringResource(R.string.about)) }
            }
        }
    }
}

/** "name · product version"; a server whose name is its product (or empty) is named once. */
internal fun foundTitle(server: FoundServer): String {
    val product = listOf(server.product, server.version).filter { it.isNotBlank() }.joinToString(" ")
    return if (server.name.isBlank() || server.name == server.product) product else "${server.name} · $product"
}
