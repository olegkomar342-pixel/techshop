-- =====================================================================
--  Презентацияға арналған сұраулар (pgAdmin → shop → Query Tool)
--  Әр сұрауды белгілеп, F5 басыңыз.
-- =====================================================================

-- 0. ДҚБЖ нұсқасы (MS SQL / MySQL-дегі SELECT @@VERSION баламасы)
SELECT version();
SHOW server_version;

-- Кестелер тізімі (7 кесте)
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public' ORDER BY table_name;

-- 1. Каталог: тауарлар санаттарымен
SELECT p.name, c.name AS category, p.price, p.stock
FROM products p LEFT JOIN categories c ON c.id = p.category_id
ORDER BY p.price DESC;

-- 2. Рейтинг: орташа баға және пікір саны
SELECT p.name, ROUND(AVG(r.rating), 1) AS rating, COUNT(*)
FROM reviews r JOIN products p ON p.id = r.product_id
GROUP BY p.name ORDER BY rating DESC;

-- 3. Тапсырыс: кім, не сатып алды, қандай күйде
SELECT o.id, u.full_name, o.status, oi.product_name, o.total
FROM orders o JOIN users u ON u.id = o.user_id
JOIN order_items oi ON oi.order_id = o.id;

-- Қосымша: сайтта тапсырыс бергеннен кейін қалдықтың азайғанын көрсету
SELECT id, name, stock FROM products ORDER BY id;

-- Қосымша: құпиясөздер тек bcrypt хэші түрінде сақталады
SELECT email, role, password_hash FROM users;
