package io.github.phantomconartist.mace.ui

import android.graphics.Bitmap
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Devices
import androidx.compose.material.icons.rounded.PhoneAndroid
import androidx.compose.material.icons.rounded.QrCodeScanner
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.AppModel
import io.github.phantomconartist.mace.Screen
import io.github.phantomconartist.mace.data.Device
import io.github.phantomconartist.mace.ui.theme.mace
import kotlinx.coroutines.launch

/**
 * Confirms the owner with the phone's fingerprint, face, or screen lock
 * before linking. Runs the action directly on a phone without a lock.
 */
val LocalVerify = compositionLocalOf<(String, () -> Unit) -> Unit> { { _, ok -> ok() } }

/** A phone linked to a computer by shared messages, drawn from MACE's shapes. */
@Composable
fun LinkArt(modifier: Modifier = Modifier) {
    val red = mace.signal
    val ink = MaterialTheme.colorScheme.onSurface
    val paper = MaterialTheme.colorScheme.surfaceContainerHigh
    Canvas(modifier.size(width = 240.dp, height = 150.dp)) {
        val u = size.width / 240f
        val stroke = Stroke(width = 2.5f * u)
        // The computer.
        drawRoundRect(red, Offset(118 * u, 18 * u), Size(104 * u, 70 * u), CornerRadius(8 * u))
        drawRoundRect(paper, Offset(126 * u, 26 * u), Size(88 * u, 54 * u), CornerRadius(4 * u))
        for (i in 0..3) drawLine(red.copy(alpha = 0.5f), Offset(134 * u, (36 + i * 11) * u), Offset((190 - i * 12) * u, (36 + i * 11) * u), strokeWidth = 3 * u)
        drawRoundRect(red, Offset(104 * u, 92 * u), Size(132 * u, 12 * u), CornerRadius(6 * u))
        // The phone.
        drawRoundRect(paper, Offset(18 * u, 30 * u), Size(58 * u, 104 * u), CornerRadius(10 * u))
        drawRoundRect(ink, Offset(18 * u, 30 * u), Size(58 * u, 104 * u), CornerRadius(10 * u), style = stroke)
        drawCircle(ink, 3 * u, Offset(47 * u, 124 * u))
        // Messages travelling between them.
        listOf(Offset(62 * u, 46 * u), Offset(88 * u, 66 * u), Offset(62 * u, 86 * u)).forEachIndexed { i, o ->
            drawRoundRect(if (i == 1) red else ink.copy(alpha = 0.85f), o, Size(34 * u, 18 * u), CornerRadius(9 * u))
        }
        drawCircle(red, 5 * u, Offset(105 * u, 75 * u))
    }
}

/** The first screen: link this phone to a hub by scanning its QR or typing its link. */
@Composable
fun LinkScreen(state: Screen.SignIn, onSignIn: (String) -> Unit, onBack: (() -> Unit)? = null) {
    if (onBack != null) BackHandler(onBack = onBack)
    val context = LocalContext.current
    val verify = LocalVerify.current
    var typing by rememberSaveable { mutableStateOf(false) }
    var link by rememberSaveable { mutableStateOf("") }
    var scanProblem by remember { mutableStateOf<String?>(null) }
    fun go(text: String) = verify("Link this phone to your hub") { onSignIn(text) }
    Box(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface).safeDrawingPadding().imePadding().padding(horizontal = 24.dp), contentAlignment = Alignment.Center) {
        Column(Modifier.widthIn(max = 440.dp).fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(16.dp)) {
            LinkArt()
            Text("Link to your hub", fontSize = 28.sp, textAlign = TextAlign.Center)
            Text(
                "On your computer, open MACE and go to Settings, Phone, or run mace-server qr. Then scan the code. Your own devices on Tailscale can type the hub's address instead.",
                style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center,
            )
            if (onBack != null) TextButton(onBack) { Text("Back to saved chats", color = MaterialTheme.colorScheme.onSurfaceVariant) }
            (state.problem ?: scanProblem)?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium, textAlign = TextAlign.Center) }
            if (typing) {
                OutlinedTextField(link, { link = it }, label = { Text("Hub address or sign-in link") }, placeholder = { Text("your-pc.your-tailnet.ts.net") },
                    singleLine = true, enabled = !state.busy, modifier = Modifier.fillMaxWidth())
                Button({ go(link) }, enabled = link.isNotBlank() && !state.busy, colors = ButtonDefaults.buttonColors(containerColor = mace.signal, contentColor = mace.onSignal),
                    modifier = Modifier.fillMaxWidth().height(52.dp)) { Text(if (state.busy) "Linking…" else "Link") }
                TextButton({ typing = false }) { Text("Scan a QR code instead", color = MaterialTheme.colorScheme.primary) }
            } else {
                Button(
                    {
                        scanProblem = null
                        com.google.mlkit.vision.codescanner.GmsBarcodeScanning.getClient(
                            context,
                            com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions.Builder().setBarcodeFormats(com.google.mlkit.vision.barcode.common.Barcode.FORMAT_QR_CODE).build(),
                        ).startScan()
                            .addOnSuccessListener { code -> code.rawValue?.let { go(it) } }
                            .addOnFailureListener { scanProblem = "The scanner could not start. Type the link instead." }
                    },
                    enabled = !state.busy,
                    colors = ButtonDefaults.buttonColors(containerColor = mace.signal, contentColor = mace.onSignal),
                    modifier = Modifier.fillMaxWidth().height(52.dp),
                ) {
                    Icon(Icons.Rounded.QrCodeScanner, null)
                    Spacer(Modifier.width(10.dp))
                    Text(if (state.busy) "Linking…" else "Scan QR code")
                }
                TextButton({ typing = true }) { Text("Type the link instead", color = MaterialTheme.colorScheme.primary) }
            }
        }
    }
}

/** A QR code for a link, drawn on the phone. */
fun qrBitmap(text: String, size: Int = 720): Bitmap {
    val matrix = com.google.zxing.qrcode.QRCodeWriter().encode(text, com.google.zxing.BarcodeFormat.QR_CODE, size, size, mapOf(com.google.zxing.EncodeHintType.MARGIN to 1))
    val bmp = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
    for (y in 0 until size) for (x in 0 until size) bmp.setPixel(x, y, if (matrix[x, y]) 0xFF171213.toInt() else 0xFFFFFDFA.toInt())
    return bmp
}

/** Linked devices, as in a messenger: link another one, see the rest, sign one out. */
@Composable
fun LinkedDevicesPage(model: AppModel, onBack: () -> Unit) {
    BackHandler(onBack = onBack)
    val verify = LocalVerify.current
    val clipboard = LocalClipboardManager.current
    val scope = rememberCoroutineScope()
    var devices by remember { mutableStateOf<List<Device>?>(null) }
    var reload by remember { mutableIntStateOf(0) }
    var link by remember { mutableStateOf<String?>(null) }
    var out by remember { mutableStateOf<Device?>(null) }
    var problem by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(reload) { devices = runCatching { model.devices() }.getOrDefault(emptyList()) }
    Scaffold(containerColor = MaterialTheme.colorScheme.surface, topBar = { PageBar("Linked devices", onBack = onBack) }) { pad ->
        LazyColumn(Modifier.padding(pad).fillMaxSize()) {
            item {
                Column(Modifier.fillMaxWidth().padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    LinkArt()
                    Spacer(Modifier.height(16.dp))
                    Text("Link a browser or another phone to this hub. Each device signs in once and can be signed out here.",
                        style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center)
                    Spacer(Modifier.height(20.dp))
                    Button(
                        {
                            verify("Link a new device") {
                                scope.launch { runCatching { model.signInLink() }.onSuccess { link = it }.onFailure { problem = it.message } }
                            }
                        },
                        colors = ButtonDefaults.buttonColors(containerColor = mace.signal, contentColor = mace.onSignal),
                        modifier = Modifier.fillMaxWidth().height(50.dp),
                    ) { Text("Link a device") }
                    problem?.let { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(top = 8.dp)) }
                }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
            }
            item {
                Column(Modifier.padding(horizontal = 20.dp, vertical = 16.dp)) {
                    Text("DEVICE STATUS", style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text("Tap a device to sign it out.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            if (devices == null) item { Text("Loading…", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 20.dp)) }
            items(devices.orEmpty(), key = { it.id }) { d ->
                Row(Modifier.fillMaxWidth().clickable(enabled = !d.current) { out = d }.padding(horizontal = 20.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Box(Modifier.size(48.dp).clip(CircleShape).background(memberColor(d.id).copy(alpha = 0.2f)), contentAlignment = Alignment.Center) {
                        Icon(if (d.name.contains("MACE on")) Icons.Rounded.PhoneAndroid else Icons.Rounded.Devices, null, tint = memberColor(d.id))
                    }
                    Spacer(Modifier.width(16.dp))
                    Column {
                        Text(d.name.ifEmpty { "Device" } + if (d.current) " (this phone)" else "", fontSize = 17.sp)
                        Text(if (d.current) "Active now" else d.lastSeen.takeIf { it.isNotEmpty() }?.let { "Last active " + listTime(it) } ?: "Signed in",
                            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        }
    }
    link?.let { l ->
        val qr = remember(l) { qrBitmap(l).asImageBitmap() }
        AlertDialog(
            onDismissRequest = { link = null; reload++ },
            title = { Text("Scan to link") },
            text = {
                Column(horizontalAlignment = Alignment.CenterHorizontally) {
                    Image(qr, "Sign-in QR code", Modifier.size(240.dp).clip(RoundedCornerShape(12.dp)))
                    Spacer(Modifier.height(12.dp))
                    Text("Scan with MACE on another phone, or open the link in a browser. It works once, for 10 minutes.", style = MaterialTheme.typography.bodyMedium, textAlign = TextAlign.Center)
                }
            },
            confirmButton = { TextButton({ clipboard.setText(AnnotatedString(l)) }) { Text("Copy link") } },
            dismissButton = { TextButton({ link = null; reload++ }) { Text("Done") } },
        )
    }
    out?.let { d -> Confirm("Sign out ${d.name}?", "It stops reaching your hub until it is linked again.", "Sign out", { model.revoke(d.id); reload++ }) { out = null } }
}
