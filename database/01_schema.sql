-- =====================================================================
--  Интернет-дүкен (электроника және аксессуарлар) — дерекқор схемасы
--  1-қадам: 7 кесте, кілттер, каскадты жою, индекстер, сынақ деректері
--
--  Іске қосу: pgAdmin → shop дерекқоры → Query Tool → осы файлды ашып, F5.
--  НАЗАР АУДАРЫҢЫЗ: файл басында кестелер ЖОЙЫЛАДЫ (DROP) — қайта іске
--  қосқанда барлық деректер сынақ деректеріне қайтарылады.
-- =====================================================================

DROP TABLE IF EXISTS reviews     CASCADE;
DROP TABLE IF EXISTS order_items CASCADE;
DROP TABLE IF EXISTS orders      CASCADE;
DROP TABLE IF EXISTS cart_items  CASCADE;
DROP TABLE IF EXISTS products    CASCADE;
DROP TABLE IF EXISTS categories  CASCADE;
DROP TABLE IF EXISTS users       CASCADE;
DROP FUNCTION IF EXISTS set_updated_at() CASCADE;

-- ---------------------------------------------------------------------
-- 1. users — сатып алушылар мен әкімшілер
-- ---------------------------------------------------------------------
CREATE TABLE users (
    id            SERIAL PRIMARY KEY,
    email         VARCHAR(255) NOT NULL UNIQUE,
    password_hash TEXT         NOT NULL,           -- тек bcrypt хэші сақталады
    full_name     VARCHAR(100) NOT NULL,
    phone         VARCHAR(30),
    role          VARCHAR(20)  NOT NULL DEFAULT 'customer'
                  CHECK (role IN ('customer', 'admin')),
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------
-- 2. categories — тауар санаттары (ішкі санаттармен)
-- ---------------------------------------------------------------------
CREATE TABLE categories (
    id        SERIAL PRIMARY KEY,
    name      VARCHAR(100) NOT NULL,
    slug      VARCHAR(100) NOT NULL UNIQUE,
    parent_id INT REFERENCES categories(id) ON DELETE SET NULL
);

-- ---------------------------------------------------------------------
-- 3. products — тауарлар
-- ---------------------------------------------------------------------
CREATE TABLE products (
    id          SERIAL PRIMARY KEY,
    category_id INT REFERENCES categories(id) ON DELETE SET NULL,
    name        VARCHAR(200)   NOT NULL,
    description TEXT           NOT NULL DEFAULT '',
    price       NUMERIC(12,2)  NOT NULL CHECK (price >= 0),
    stock       INT            NOT NULL DEFAULT 0 CHECK (stock >= 0),
    image_url   TEXT,
    is_active   BOOLEAN        NOT NULL DEFAULT TRUE,  -- FALSE → каталогтан жасырылған
    created_at  TIMESTAMPTZ    NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ    NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------
-- 4. cart_items — пайдаланушы себеті
-- ---------------------------------------------------------------------
CREATE TABLE cart_items (
    user_id    INT NOT NULL REFERENCES users(id)    ON DELETE CASCADE,
    product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    quantity   INT NOT NULL CHECK (quantity > 0),
    added_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, product_id)
);

-- ---------------------------------------------------------------------
-- 5. orders — тапсырыстар
-- ---------------------------------------------------------------------
CREATE TABLE orders (
    id               SERIAL PRIMARY KEY,
    user_id          INT REFERENCES users(id) ON DELETE SET NULL,
    status           VARCHAR(20) NOT NULL DEFAULT 'new'
                     CHECK (status IN ('new', 'paid', 'shipped', 'delivered', 'cancelled')),
    full_name        VARCHAR(100)  NOT NULL,
    phone            VARCHAR(30)   NOT NULL,
    shipping_address TEXT          NOT NULL,
    delivery_method  VARCHAR(20)   NOT NULL DEFAULT 'courier'
                     CHECK (delivery_method IN ('courier', 'pickup')),
    payment_method   VARCHAR(20)   NOT NULL DEFAULT 'cash'
                     CHECK (payment_method IN ('cash', 'card')),
    delivery_cost    NUMERIC(12,2) NOT NULL DEFAULT 0 CHECK (delivery_cost >= 0),
    total            NUMERIC(12,2) NOT NULL CHECK (total >= 0),  -- тауарлар + жеткізу
    comment          TEXT          NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ   NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------
-- 6. order_items — тапсырыс позициялары
--    атауы мен бағасы сатып алу сәтінде көшіріліп сақталады
-- ---------------------------------------------------------------------
CREATE TABLE order_items (
    id           SERIAL PRIMARY KEY,
    order_id     INT NOT NULL REFERENCES orders(id)   ON DELETE CASCADE,
    product_id   INT          REFERENCES products(id) ON DELETE SET NULL,
    product_name VARCHAR(200)  NOT NULL,
    unit_price   NUMERIC(12,2) NOT NULL CHECK (unit_price >= 0),
    quantity     INT           NOT NULL CHECK (quantity > 0)
);

-- ---------------------------------------------------------------------
-- 7. reviews — пікірлер мен бағалар (1–5), бір тауарға бір пікір
-- ---------------------------------------------------------------------
CREATE TABLE reviews (
    id         SERIAL PRIMARY KEY,
    product_id INT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id    INT          REFERENCES users(id)    ON DELETE SET NULL,
    rating     SMALLINT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment    TEXT     NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (product_id, user_id)
);

-- ---------------------------------------------------------------------
-- updated_at триггері (products, orders)
-- ---------------------------------------------------------------------
CREATE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_products_updated_at BEFORE UPDATE ON products
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER trg_orders_updated_at BEFORE UPDATE ON orders
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- ---------------------------------------------------------------------
-- Индекстер
-- ---------------------------------------------------------------------
-- Толық мәтінді іздеу (атауы + сипаттамасы). 'simple' — тілге тәуелсіз,
-- сондықтан қазақша, орысша және ағылшынша сөздерге бірдей жұмыс істейді.
CREATE INDEX idx_products_search ON products
    USING GIN (to_tsvector('simple', name || ' ' || description));
CREATE INDEX idx_products_category_price ON products (category_id, price);
CREATE INDEX idx_products_price          ON products (price);
CREATE INDEX idx_orders_user_created     ON orders (user_id, created_at DESC);  -- «менің тапсырыстарым»
CREATE INDEX idx_orders_status           ON orders (status);
CREATE INDEX idx_order_items_order       ON order_items (order_id);
CREATE INDEX idx_reviews_product         ON reviews (product_id);
CREATE INDEX idx_categories_parent       ON categories (parent_id);

-- =====================================================================
--  Сынақ деректері: 3 пайдаланушы, 4 санат, 8 тауар, себет, 1 тапсырыс, 3 пікір
--  Құпиясөздер (тек сынақ үшін!):  admin@shop.kz / admin123
--                                  aruzhan@mail.kz, daniyar@mail.kz / password123
-- =====================================================================
INSERT INTO users (email, password_hash, full_name, phone, role) VALUES
 ('admin@shop.kz',   '$2a$10$J9hwiNXU/L6i5flXqeaBs.9/q3k/eoDI6HebjOR8CzoYKX6elNRHu', 'Әкімші',          '+7 700 000 0001', 'admin'),
 ('aruzhan@mail.kz', '$2a$10$U/mFE1XTvdQLUYsp33brNOYlA43IEiBhJ/1oUQhGDkv39EoQJy2ne', 'Аружан Сейітова', '+7 701 111 2233', 'customer'),
 ('daniyar@mail.kz', '$2a$10$U/mFE1XTvdQLUYsp33brNOYlA43IEiBhJ/1oUQhGDkv39EoQJy2ne', 'Данияр Омаров',   '+7 702 444 5566', 'customer');

INSERT INTO categories (name, slug, parent_id) VALUES
 ('Смартфондар',   'smartphones', NULL),
 ('Ноутбуктар',    'laptops',     NULL),
 ('Аксессуарлар',  'accessories', NULL),
 ('Құлаққаптар',   'headphones',  3);    -- «Аксессуарлар» ішкі санаты

INSERT INTO products (category_id, name, description, price, stock) VALUES
 (1, 'Samsung Galaxy S24',      '6.2" Dynamic AMOLED экран, 8/256 ГБ, 50 Мп камера, 4000 мА·сағ батарея.',       449990.00, 12),
 (1, 'Apple iPhone 15',         '6.1" Super Retina XDR, A16 Bionic чипі, 128 ГБ, USB-C порты.',                   529990.00,  7),
 (1, 'Xiaomi Redmi Note 13',    '6.67" AMOLED 120 Гц, 8/256 ГБ, 108 Мп камера, 5000 мА·сағ, жылдам зарядтау.',    119990.00, 25),
 (2, 'Lenovo IdeaPad Slim 5',   '14" OLED, AMD Ryzen 7, 16 ГБ RAM, 512 ГБ SSD — оқуға және жұмысқа арналған ноутбук.', 389990.00, 5),
 (2, 'Apple MacBook Air M2',    '13.6" Liquid Retina, Apple M2, 8 ГБ, 256 ГБ SSD, 18 сағатқа дейін жұмыс.',       569990.00,  3),
 (3, 'Anker PowerCore 20000',   'Сыртқы аккумулятор 20000 мА·сағ, екі USB порты, жылдам зарядтау.',               19990.00, 40),
 (4, 'Apple AirPods Pro 2',     'Белсенді шуды басу, USB-C кейсі, кеңістіктік дыбыс.',                           129990.00, 10),
 (4, 'JBL Tune 520BT',          'Сымсыз толық өлшемді құлаққап, Bluetooth 5.3, 57 сағатқа дейін жұмыс.',          24990.00,  1);

-- Себет: Аружанда 2 тауар
INSERT INTO cart_items (user_id, product_id, quantity) VALUES
 (2, 6, 2),
 (2, 3, 1);

-- 1 тапсырыс: Данияр (AirPods + қуат банкі), жеткізу тегін (50 000 ₸-ден жоғары)
INSERT INTO orders (user_id, status, full_name, phone, shipping_address, delivery_method, payment_method, delivery_cost, total)
VALUES (3, 'paid', 'Данияр Омаров', '+7 702 444 5566', 'Алматы қ., Абай даңғ. 10, 25 пәтер', 'courier', 'card', 0, 149980.00);

INSERT INTO order_items (order_id, product_id, product_name, unit_price, quantity) VALUES
 (1, 7, 'Apple AirPods Pro 2',   129990.00, 1),
 (1, 6, 'Anker PowerCore 20000',  19990.00, 1);

INSERT INTO reviews (product_id, user_id, rating, comment) VALUES
 (7, 3, 5, 'Шуды басуы керемет, батареясы ұзақ ұстайды.'),
 (6, 3, 4, 'Ауыр болғанымен, телефонды 4 рет толық зарядтайды.'),
 (3, 2, 5, 'Бағасына қарағанда өте жақсы смартфон!');
