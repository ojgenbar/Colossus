import 'dart:async';
import 'dart:convert';
// ignore: avoid_web_libraries_in_flutter
import 'dart:html' as html;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:file_picker/file_picker.dart';
import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart';

void main() {
  runApp(const ColossusApp());
}

class ColossusApp extends StatelessWidget {
  const ColossusApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Colossus',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorSchemeSeed: const Color(0xFF1565C0),
        useMaterial3: true,
        brightness: Brightness.dark,
      ),
      home: const UploadPage(),
    );
  }
}

class UploadPage extends StatefulWidget {
  const UploadPage({super.key});

  @override
  State<UploadPage> createState() => _UploadPageState();
}

enum ProcessingStatus { idle, uploading, polling, done, error }

class _ImageResult {
  final String rawUrl;
  final String processedUrl;

  _ImageResult({required this.rawUrl, required this.processedUrl});
}

class _UploadPageState extends State<UploadPage> {
  ProcessingStatus _status = ProcessingStatus.idle;
  String _message = '';
  _ImageResult? _result;
  Timer? _pollTimer;
  final _uuidController = TextEditingController();
  final _extController = TextEditingController(text: 'jpg');

  @override
  void dispose() {
    _pollTimer?.cancel();
    _uuidController.dispose();
    _extController.dispose();
    super.dispose();
  }

  void _showImages(_ImageResult result, {bool poll = false}) {
    setState(() {
      _result = result;
      if (poll) {
        _status = ProcessingStatus.polling;
        _message = 'Processing image...';
      } else {
        _status = ProcessingStatus.done;
        _message = '';
      }
    });
    if (poll) _startPolling(result.processedUrl);
  }

  Future<void> _pickAndUpload() async {
    final picked = await FilePicker.platform.pickFiles(
      type: FileType.image,
      withData: true,
    );
    if (picked == null || picked.files.isEmpty) return;

    final file = picked.files.first;
    if (file.bytes == null) return;

    setState(() {
      _status = ProcessingStatus.uploading;
      _message = 'Uploading ${file.name}...';
      _result = null;
    });

    try {
      final request =
          http.MultipartRequest('POST', Uri.parse('/api/upload-image'));
      request.files.add(http.MultipartFile.fromBytes(
        'file',
        file.bytes!,
        filename: file.name,
        contentType: MediaType('image', file.extension ?? 'jpeg'),
      ));

      final streamed = await request.send();
      final response = await http.Response.fromStream(streamed);

      if (response.statusCode != 200) {
        setState(() {
          _status = ProcessingStatus.error;
          _message = 'Upload failed: ${response.statusCode} ${response.body}';
        });
        return;
      }

      final body = jsonDecode(response.body) as Map<String, dynamic>;
      final rawFilename = body['filename_raw'] as String;
      final processedFilename = body['filename_processed'] as String;

      _showImages(
        _ImageResult(
          rawUrl: '/api/retrieve-image/raw/$rawFilename',
          processedUrl: '/api/retrieve-image/processed/$processedFilename',
        ),
        poll: true,
      );
    } catch (e) {
      setState(() {
        _status = ProcessingStatus.error;
        _message = 'Error: $e';
      });
    }
  }

  void _lookupByUuid() {
    final uuid = _uuidController.text.trim();
    if (uuid.isEmpty) return;
    final ext = _extController.text.trim();
    if (ext.isEmpty) return;

    _pollTimer?.cancel();

    final rawUrl = '/api/retrieve-image/raw/$uuid-raw.$ext';
    final processedUrl = '/api/retrieve-image/processed/$uuid-processed.$ext';

    // Check if processed exists already
    setState(() {
      _status = ProcessingStatus.polling;
      _message = 'Looking up $uuid...';
      _result = null;
    });

    http.get(Uri.parse(processedUrl)).then((resp) {
      if (resp.statusCode == 200) {
        _showImages(
            _ImageResult(rawUrl: rawUrl, processedUrl: processedUrl));
      } else {
        _showImages(
          _ImageResult(rawUrl: rawUrl, processedUrl: processedUrl),
          poll: true,
        );
      }
    }).catchError((_) {
      _showImages(
        _ImageResult(rawUrl: rawUrl, processedUrl: processedUrl),
        poll: true,
      );
    });
  }

  void _startPolling(String processedUrl) {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 2), (timer) async {
      try {
        final resp = await http.get(Uri.parse(processedUrl));
        if (resp.statusCode == 200) {
          timer.cancel();
          setState(() {
            _status = ProcessingStatus.done;
            _message = 'Processing complete!';
          });
        }
      } catch (_) {
        // keep polling
      }
    });
  }

  void _copyToClipboard(String url) {
    final fullUrl = '${html.window.location.origin}$url';
    Clipboard.setData(ClipboardData(text: fullUrl));
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text('Copied: $fullUrl'),
        duration: const Duration(seconds: 2),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Colossus'),
        centerTitle: true,
      ),
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 960),
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.center,
              children: [
                _buildActions(),
                if (_status != ProcessingStatus.idle) ...[
                  const SizedBox(height: 16),
                  _buildStatusBar(),
                ],
                if (_result != null) ...[
                  const SizedBox(height: 24),
                  Expanded(child: _buildImageComparison()),
                ],
              ],
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildActions() {
    final busy = _status == ProcessingStatus.uploading ||
        _status == ProcessingStatus.polling;

    return Row(
      children: [
        FilledButton.icon(
          onPressed: busy ? null : _pickAndUpload,
          icon: const Icon(Icons.upload_file),
          label: const Text('Upload Image'),
        ),
        const SizedBox(width: 24),
        const Text('or', style: TextStyle(fontSize: 14)),
        const SizedBox(width: 24),
        SizedBox(
          width: 300,
          child: TextField(
            controller: _uuidController,
            decoration: const InputDecoration(
              labelText: 'UUID',
              hintText: 'e.g. 3f2504e0-4f89-11d3-9a0c-0305e82c3301',
              border: OutlineInputBorder(),
              isDense: true,
            ),
            onSubmitted: (_) => _lookupByUuid(),
          ),
        ),
        const SizedBox(width: 8),
        SizedBox(
          width: 70,
          child: TextField(
            controller: _extController,
            decoration: const InputDecoration(
              labelText: 'Ext',
              border: OutlineInputBorder(),
              isDense: true,
            ),
            onSubmitted: (_) => _lookupByUuid(),
          ),
        ),
        const SizedBox(width: 8),
        FilledButton.tonal(
          onPressed: busy ? null : _lookupByUuid,
          child: const Text('Lookup'),
        ),
      ],
    );
  }

  Widget _buildStatusBar() {
    final isLoading = _status == ProcessingStatus.uploading ||
        _status == ProcessingStatus.polling;
    final isError = _status == ProcessingStatus.error;

    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        if (isLoading) ...[
          const SizedBox(
            width: 16,
            height: 16,
            child: CircularProgressIndicator(strokeWidth: 2),
          ),
          const SizedBox(width: 12),
        ],
        if (isError) ...[
          const Icon(Icons.error_outline, color: Colors.red, size: 18),
          const SizedBox(width: 8),
        ],
        if (_status == ProcessingStatus.done) ...[
          const Icon(Icons.check_circle_outline, color: Colors.green, size: 18),
          const SizedBox(width: 8),
        ],
        Flexible(child: Text(_message)),
      ],
    );
  }

  Widget _buildImageComparison() {
    final result = _result!;
    final showProcessed = _status == ProcessingStatus.done;

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(child: _buildImageCard('Original', result.rawUrl)),
        const SizedBox(width: 16),
        Expanded(
          child: showProcessed
              ? _buildImageCard('Processed', result.processedUrl)
              : const Card(
                  child: Center(
                    child: Padding(
                      padding: EdgeInsets.all(48),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          CircularProgressIndicator(),
                          SizedBox(height: 16),
                          Text('Processing...'),
                        ],
                      ),
                    ),
                  ),
                ),
        ),
      ],
    );
  }

  Widget _buildImageCard(String label, String url) {
    return Card(
      clipBehavior: Clip.antiAlias,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 12, 8, 8),
            child: Row(
              children: [
                Text(label, style: Theme.of(context).textTheme.titleMedium),
                const Spacer(),
                IconButton(
                  icon: const Icon(Icons.copy, size: 18),
                  tooltip: 'Copy URL',
                  onPressed: () => _copyToClipboard(url),
                ),
                IconButton(
                  icon: const Icon(Icons.open_in_new, size: 18),
                  tooltip: 'Open in new tab',
                  onPressed: () => html.window.open(url, '_blank'),
                ),
              ],
            ),
          ),
          Image.network(
            url,
            fit: BoxFit.contain,
            errorBuilder: (_, __, ___) => const Padding(
              padding: EdgeInsets.all(32),
              child: Icon(Icons.broken_image, size: 48),
            ),
          ),
        ],
      ),
    );
  }
}
